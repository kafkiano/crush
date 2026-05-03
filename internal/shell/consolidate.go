package shell

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/db"
	"mvdan.cc/sh/v3/interp"
)

const consolidateUsage = `crush-consolidate — Extract session data from crush SQLite DB

Synopsis:
  %% crush-consolidate --last --verbose
  %% crush-consolidate --since 2h
  %% crush-consolidate --stats

Usage:
  crush-consolidate [OPTIONS]

Options:
  --last                    Extract most recent session (default)
  --session <id>            Extract specific session
  --all-unconsolidated      Extract all sessions since last consolidation marker
  --since <duration>        Extract sessions updated within duration (e.g. 2h, 1d, 30m)
  --stats                   Show session statistics without message content
  --max-chars <n>           Truncate output at N characters (default: 30000)
  --verbose                 Include tool result content (default: summaries only)
  -h, --help                Display this help

Duration format for --since: <number><unit> where unit is s/m/h/d
  Examples: 30m, 2h, 1d, 90s

Output: Structured text suitable for agent review and memory consolidation.
`

// MessagePart represents a single part of a message (text, tool_call, tool_result, etc.)
type MessagePart struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// handleConsolidate implements the crush-consolidate builtin. It extracts session
// data from the crush SQLite database and formats it for memory consolidation.
func handleConsolidate(args []string, cwd string, stdin io.Reader, stdout, stderr io.Writer) error {
	// Parse flags
	mode := "last"
	sessionID := ""
	maxChars := 30000
	verbose := false
	statsOnly := false
	var sinceDuration time.Duration

	i := 1 // skip "crush-consolidate"
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			fmt.Fprint(stdout, consolidateUsage)
			return nil
		case arg == "--last":
			mode = "last"
		case arg == "--session":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "crush-consolidate: --session requires an ID")
				return interp.ExitStatus(2)
			}
			mode = "session"
			sessionID = args[i+1]
			i++
		case arg == "--all-unconsolidated":
			mode = "all"
		case arg == "--since":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "crush-consolidate: --since requires a duration")
				return interp.ExitStatus(2)
			}
			d, err := parseSinceDuration(args[i+1])
			if err != nil {
				fmt.Fprintf(stderr, "crush-consolidate: invalid --since duration: %v\n", err)
				return interp.ExitStatus(2)
			}
			sinceDuration = d
			mode = "since"
			i++
		case arg == "--stats":
			statsOnly = true
		case arg == "--max-chars":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "crush-consolidate: --max-chars requires a number")
				return interp.ExitStatus(2)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(stderr, "crush-consolidate: invalid max-chars value: %s\n", args[i+1])
				return interp.ExitStatus(2)
			}
			maxChars = n
			i++
		case arg == "--verbose":
			verbose = true
		default:
			fmt.Fprintf(stderr, "crush-consolidate: unknown option: %s\n", arg)
			return interp.ExitStatus(2)
		}
		i++
	}

	// Connect to database
	dataDir := filepath.Join(cwd, ".crush")
	dbPath := filepath.Join(dataDir, "crush.db")

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(stderr, "crush-consolidate: crush.db not found at %s\n", dbPath)
		return interp.ExitStatus(1)
	}

	ctx := context.Background()
	sqlDB, err := db.Connect(ctx, dataDir)
	if err != nil {
		fmt.Fprintf(stderr, "crush-consolidate: failed to connect to database: %v\n", err)
		return interp.ExitStatus(1)
	}
	defer sqlDB.Close()

	queries := db.New(sqlDB)

	// Resolve session IDs based on mode
	var sessionIDs []string
	switch mode {
	case "last":
		session, err := queries.GetLastSession(ctx)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Fprintln(stderr, "crush-consolidate: no session found")
			} else {
				fmt.Fprintf(stderr, "crush-consolidate: failed to get last session: %v\n", err)
			}
			return interp.ExitStatus(1)
		}
		sessionIDs = []string{session.ID}
	case "session":
		session, err := queries.GetSessionByID(ctx, sessionID)
		if err != nil {
			if err == sql.ErrNoRows {
				fmt.Fprintf(stderr, "crush-consolidate: session not found: %s\n", sessionID)
			} else {
				fmt.Fprintf(stderr, "crush-consolidate: failed to get session: %v\n", err)
			}
			return interp.ExitStatus(1)
		}
		sessionIDs = []string{session.ID}
	case "since":
		cutoff := time.Now().Add(-sinceDuration).Unix()
		sessions, err := queries.ListSessions(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "crush-consolidate: failed to list sessions: %v\n", err)
			return interp.ExitStatus(1)
		}
		for _, s := range sessions {
			if s.UpdatedAt >= cutoff {
				sessionIDs = append(sessionIDs, s.ID)
			}
		}
		if len(sessionIDs) == 0 {
			fmt.Fprintf(stdout, "No sessions found since %s.\n", time.Now().Add(-sinceDuration).Format("2006-01-02 15:04:05"))
			return nil
		}
	case "all":
		// Read the last consolidated marker
		markerPath := filepath.Join(dataDir, "memory", "last-consolidated")
		var marker int64 = 0
		if data, err := os.ReadFile(markerPath); err == nil {
			if m, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
				marker = m
			}
		}

		// Get all sessions since the marker
		sessions, err := queries.ListSessions(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "crush-consolidate: failed to list sessions: %v\n", err)
			return interp.ExitStatus(1)
		}

		for _, session := range sessions {
			if session.UpdatedAt > marker {
				sessionIDs = append(sessionIDs, session.ID)
			}
		}

		if len(sessionIDs) == 0 {
			fmt.Fprintln(stdout, "No unconsolidated sessions found.")
			return nil
		}
	}

	// Stats mode: print summary and return
	if statsOnly {
		return printStats(ctx, queries, sessionIDs, stdout, stderr)
	}

	// Format and output sessions
	totalChars := 0
	truncated := false
	for _, sid := range sessionIDs {
		if truncated {
			break
		}

		session, err := queries.GetSessionByID(ctx, sid)
		if err != nil {
			fmt.Fprintf(stderr, "crush-consolidate: failed to get session %s: %v\n", sid, err)
			continue
		}

		// Format and print session header immediately
		createdAt := time.Unix(session.CreatedAt, 0).Format("2006-01-02 15:04:05")
		header := fmt.Sprintf("=== SESSION: %q (%s) ===\n  messages: %d | tokens: %d in / %d out\n",
			session.Title,
			createdAt,
			session.MessageCount,
			session.PromptTokens,
			session.CompletionTokens,
		)

		if totalChars+len(header) > maxChars {
			fmt.Fprintf(stdout, "--- TRUNCATED: max chars (%d) reached ---\n", maxChars)
			truncated = true
			break
		}
		fmt.Fprint(stdout, header)
		fmt.Fprint(stdout, "\n")
		totalChars += len(header) + 1

		// Get messages
		messages, err := queries.ListMessagesBySession(ctx, sid)
		if err != nil {
			fmt.Fprintf(stderr, "crush-consolidate: failed to get messages for session %s: %v\n", sid, err)
			continue
		}

		// Format and print messages incrementally
		for _, msg := range messages {
			formatted := formatMessage(msg, verbose)
			if totalChars+len(formatted) > maxChars {
				fmt.Fprintf(stdout, "--- TRUNCATED: max chars (%d) reached ---\n", maxChars)
				truncated = true
				break
			}
			fmt.Fprint(stdout, formatted)
			totalChars += len(formatted)
		}

		if !truncated {
			fmt.Fprint(stdout, "\n")
			totalChars++
		}
	}

	return nil
}

// formatMessage formats a single message according to the consolidation rules
func formatMessage(msg db.Message, verbose bool) string {
	var parts []MessagePart
	if err := json.Unmarshal([]byte(msg.Parts), &parts); err != nil {
		// If JSON parsing fails, try treating it as a string
		return fmt.Sprintf("[%s] %s\n", strings.ToUpper(msg.Role), msg.Parts)
	}

	var output strings.Builder

	switch msg.Role {
	case "user":
		// User messages: [USER] <text truncated to 200 chars>
		for _, part := range parts {
			if part.Type == "text" {
				if text, ok := part.Data["text"].(string); ok {
					truncated := truncateString(text, 200)
					output.WriteString(fmt.Sprintf("[USER] %s\n", truncated))
					break
				}
			}
		}

	case "assistant":
		// Assistant text: [ASSISTANT] <text truncated to 400 chars>
		var textParts []string
		for _, part := range parts {
			if part.Type == "text" {
				if text, ok := part.Data["text"].(string); ok {
					textParts = append(textParts, text)
				}
			}
		}
		if len(textParts) > 0 {
			combined := strings.Join(textParts, "\n")
			truncated := truncateString(combined, 400)
			output.WriteString(fmt.Sprintf("[ASSISTANT] %s\n", truncated))
		}

		// Tool calls: [TOOL_CALL] <name> (<input truncated to 80 chars>)
		for _, part := range parts {
			if part.Type == "tool_call" {
				name := ""
				if n, ok := part.Data["name"].(string); ok {
					name = n
				}
				inputStr := ""
				if input, ok := part.Data["input"]; ok {
					inputBytes, _ := json.Marshal(input)
					inputStr = truncateString(string(inputBytes), 80)
				}
				if inputStr != "" {
					output.WriteString(fmt.Sprintf("[TOOL_CALL] %s (%s)\n", name, inputStr))
				} else {
					output.WriteString(fmt.Sprintf("[TOOL_CALL] %s\n", name))
				}
			}
		}

		// Finish reason: [FINISH] <reason>
		for _, part := range parts {
			if part.Type == "finish" {
				if reason, ok := part.Data["reason"].(string); ok {
					output.WriteString(fmt.Sprintf("[FINISH] %s\n", reason))
				}
			}
		}

	case "tool":
		// Tool results
		for _, part := range parts {
			if part.Type == "tool_result" {
				name := ""
				if n, ok := part.Data["name"].(string); ok {
					name = n
				}
				isError := false
				if ie, ok := part.Data["is_error"].(bool); ok {
					isError = ie
				}
				content := ""
				if c, ok := part.Data["content"].(string); ok {
					content = c
				}

				if isError {
					output.WriteString(fmt.Sprintf("[TOOL_RESULT] %s (ERROR)", name))
				} else {
					output.WriteString(fmt.Sprintf("[TOOL_RESULT] %s", name))
				}

				if content != "" {
					if verbose {
						truncated := truncateString(content, 200)
						output.WriteString(fmt.Sprintf(" → %s\n", truncated))
					} else {
						lines := strings.Count(content, "\n") + 1
						output.WriteString(fmt.Sprintf(" → %d lines\n", lines))
					}
				} else {
					output.WriteString(" → (binary/no content)\n")
				}
			}
		}
	}

	return output.String()
}

// truncateString truncates a string to maxLen and adds "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseSinceDuration parses a duration string like "2h", "30m", "1d", "90s".
func parseSinceDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration: %q", s)
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number in duration: %q", s)
	}
	switch unit {
	case 's':
		return time.Duration(num) * time.Second, nil
	case 'm':
		return time.Duration(num) * time.Minute, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %c (use s/m/h/d)", unit)
	}
}

// printStats outputs a summary of sessions without message content.
func printStats(ctx context.Context, queries *db.Queries, sessionIDs []string, stdout, stderr io.Writer) error {
	fmt.Fprintf(stdout, "Sessions: %d\n\n", len(sessionIDs))

	var totalMsgs, totalIn, totalOut int64
	toolCounts := make(map[string]int)

	for _, sid := range sessionIDs {
		session, err := queries.GetSessionByID(ctx, sid)
		if err != nil {
			fmt.Fprintf(stderr, "crush-consolidate: failed to get session %s: %v\n", sid, err)
			continue
		}

		createdAt := time.Unix(session.CreatedAt, 0).Format("2006-01-02 15:04:05")
		fmt.Fprintf(stdout, "=== %q (%s) ===\n", session.Title, createdAt)
		fmt.Fprintf(stdout, "  messages: %d | tokens: %d in / %d out\n",
			session.MessageCount, session.PromptTokens, session.CompletionTokens)

		totalMsgs += session.MessageCount
		totalIn += session.PromptTokens
		totalOut += session.CompletionTokens

		messages, err := queries.ListMessagesBySession(ctx, sid)
		if err != nil {
			continue
		}

		for _, msg := range messages {
			var parts []MessagePart
			if err := json.Unmarshal([]byte(msg.Parts), &parts); err != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == "tool_call" {
					if name, ok := part.Data["name"].(string); ok {
						toolCounts[name]++
					}
				}
			}
		}
	}

	fmt.Fprintf(stdout, "\n--- Totals ---\n")
	fmt.Fprintf(stdout, "messages: %d | tokens: %d in / %d out | sessions: %d\n",
		totalMsgs, totalIn, totalOut, len(sessionIDs))

	if len(toolCounts) > 0 {
		fmt.Fprintf(stdout, "\n--- Tool Usage ---\n")
		type kv struct {
			Key   string
			Value int
		}
		var sorted []kv
		for k, v := range toolCounts {
			sorted = append(sorted, kv{k, v})
		}
		slices.SortFunc(sorted, func(a, b kv) int { return cmp.Compare(b.Value, a.Value) })
		for _, item := range sorted {
			fmt.Fprintf(stdout, "  %s: %d\n", item.Key, item.Value)
		}
	}

	return nil
}
