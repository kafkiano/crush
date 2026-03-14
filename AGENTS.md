# Crush Development Guide

## Project Overview

Crush is a terminal-based AI coding assistant built in Go by
[Charm](https://charm.land). It connects to LLMs and gives them tools to read,
write, and execute code. It supports multiple providers (Anthropic, OpenAI,
Gemini, Bedrock, Copilot, Hyper, MiniMax, Vercel, and more), integrates with
LSPs for code intelligence, and supports extensibility via MCP servers and
agent skills.

The module path is `github.com/charmbracelet/crush`.

## Architecture

```
main.go                            CLI entry point (cobra via internal/cmd)
internal/
  app/app.go                       Top-level wiring: DB, config, agents, LSP, MCP, events
  cmd/                             CLI commands (root, run, login, models, stats, sessions)
  config/
    config.go                      Config struct, context file paths, agent definitions
    load.go                        crush.json loading and validation
    provider.go                    Provider configuration and model resolution
  agent/
    agent.go                       SessionAgent: runs LLM conversations per session
    coordinator.go                 Coordinator: manages named agents ("architect", "coder", "task")
    prompts.go                     Loads Go-template system prompts
    template_loader.go             Template loading from disk or embedded fallbacks
    prompt/prompt.go               Prompt building with context injection
    templates/                     System prompt templates (architect.md.tpl, coder.md.tpl, etc.)
    tools/                         All built-in tools (bash, edit, view, grep, glob, etc.)
      mcp/                         MCP client integration
  session/session.go               Session CRUD backed by SQLite
  message/                         Message model and content types
  db/                              SQLite via sqlc, with migrations
    sql/                           Raw SQL queries (consumed by sqlc)
    migrations/                    Schema migrations
  lsp/                             LSP client manager, auto-discovery, on-demand startup
  ui/                              Bubble Tea v2 TUI (see internal/ui/AGENTS.md)
  permission/                      Tool permission checking and allow-lists
  skills/                          Skill file discovery and loading
  shell/                           Bash command execution with background job support
  event/                           Telemetry (PostHog)
  pubsub/                          Internal pub/sub for cross-component messaging
  filetracker/                     Tracks files touched per session
  history/                         Prompt history
```

## Model → Agent Architecture

Crush uses a **model → agent → template** hierarchy to route LLM requests:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLI Entry Point                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│  ./crush          → Interactive TUI  → Architect Agent → architect.md.tpl   │
│  ./crush run      → Non-interactive → Coder Agent     → coder.md.tpl        │
│  ./crush (subagent) → Task Agent      → task.md.tpl                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Agent Configuration                                │
│  internal/config/config.go:SetupAgents()                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│  AgentArchitect:                                                             │
│    Model: SelectedModelTypeLarge (e.g., claude-sonnet-4, gpt-4o)            │
│    Tools: ALL tools (bash, edit, view, glob, grep, etc.)                    │
│    MCP:  All configured MCP servers                                          │
│                                                                              │
│  AgentCoder:                                                                 │
│    Model: SelectedModelTypeLarge                                             │
│    Tools: ALL tools                                                          │
│    MCP:  All configured MCP servers                                          │
│                                                                              │
│  AgentTask:                                                                  │
│    Model: SelectedModelTypeLarge                                             │
│    Tools: READ-ONLY (glob, grep, ls, sourcegraph, view)                     │
│    MCP:  NONE (isolated search agent)                                        │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Model Resolution                                   │
│  internal/config/config.go:GetModelByType()                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  SelectedModelTypeLarge  → cfg.Models["large"] → provider/model lookup      │
│  SelectedModelTypeSmall  → cfg.Models["small"] → provider/model lookup      │
│                                                                              │
│  Example crush.json:                                                         │
│    "models": {                                                               │
│      "large": { "provider": "anthropic", "model": "claude-sonnet-4-20250514" },
│      "small": { "provider": "openai", "model": "gpt-4.1-mini" }             │
│    }                                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Agent Types

| Agent | Constant | Template | Mode | Tools | Use Case |
|-------|----------|----------|------|-------|----------|
| **Architect** | `AgentArchitect` | `architect.md.tpl` | Interactive TUI | Full | Primary user-facing agent for interactive sessions |
| **Coder** | `AgentCoder` | `coder.md.tpl` | Non-interactive (`crush run`) | Full | Headless execution for CI/CD, scripts |
| **Task** | `AgentTask` | `task.md.tpl` | Sub-agent | Read-only | Parallel search/context gathering |

### Key Implementation Files

- [`internal/config/config.go:65-68`](internal/config/config.go:65) — Agent constants
- [`internal/config/config.go:519-553`](internal/config/config.go:519) — `SetupAgents()` configuration
- [`internal/agent/coordinator.go:93-134`](internal/agent/coordinator.go:93) — `NewCoordinator()` uses architect agent
- [`internal/agent/prompts.go`](internal/agent/prompts.go) — Prompt builder functions
- [`internal/agent/template_loader.go`](internal/agent/template_loader.go) — Template loading with disk override

## Template System

### Template Loading Order

Templates are loaded from disk first, falling back to embedded templates:

1. **Disk paths** (in order):
   - `~/.config/crush/templates/{name}.md.tpl`
   - `./.crush/templates/{name}.md.tpl`

2. **Embedded fallback**:
   - `internal/agent/templates/{name}.md.tpl`

### Template Data Injection

Templates receive [`prompt.PromptDat`](internal/agent/prompt/prompt.go:29) at render time:

```go
type PromptDat struct {
    Provider      string
    Model         string
    Config        config.Config
    WorkingDir    string
    IsGitRepo     bool
    Platform      string
    Date          string
    GitStatus     string
    ContextFiles  []ContextFile  // AGENTS.md, CRUSH.md, etc.
    AvailSkillXML string         // Available skill definitions
}
```

### Custom Templates

To override a template, create a file in either template path:

```bash
# Override architect template
mkdir -p ~/.config/crush/templates
cat > ~/.config/crush/templates/architect.md.tpl << 'EOF'
# Custom Architect Prompt
Your custom instructions here...
{{range .ContextFiles}}
<memory path="{{.Path}}">
{{.Content}}
</memory>
{{end}}
EOF
```

## Debug & Prompt Debugging

### Enable Debug Logging

```bash
# Interactive mode with debug
./crush --debug

# Non-interactive with verbose output
./crush run --verbose "your prompt"
```

### View Logs

```bash
# Tail recent logs
./crush logs --tail 100

# Follow logs in real-time
./crush logs --follow

# Logs location
cat ~/.crush/logs/crush.log
```

### Debug System Prompts

To inspect the rendered system prompt:

1. Add a marker to your custom template:
   ```bash
   echo "# MARKER: CUSTOM_ARCHITECT_001" > ~/.config/crush/templates/architect.md.tpl
   ```

2. Run crush and check logs:
   ```bash
   ./crush --debug
   crush logs --tail 100 | grep "MARKER"
   ```

3. The system prompt is logged when the agent initializes

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `CRUSH_UI_DEBUG=true` | Visual TUI rerender debugging (flashing rectangles) |
| `CRUSH_DATA_DIR` | Override data directory location |

### Key Dependency Roles

- **`charm.land/fantasy`**: LLM provider abstraction layer. Handles protocol
  differences between Anthropic, OpenAI, Gemini, etc. Used in `internal/app`
  and `internal/agent`.
- **`charm.land/bubbletea/v2`**: TUI framework powering the interactive UI.
- **`charm.land/lipgloss/v2`**: Terminal styling.
- **`charm.land/glamour/v2`**: Markdown rendering in the terminal.
- **`charm.land/catwalk`**: Snapshot/golden-file testing for TUI components.
- **`sqlc`**: Generates Go code from SQL queries in `internal/db/sql/`.

### Key Patterns

- **Config is a Service**: accessed via `config.Service`, not global state.
- **Tools are self-documenting**: each tool has a `.go` implementation and a
  `.md` description file in `internal/agent/tools/`.
- **System prompts are Go templates**: `internal/agent/templates/*.md.tpl`
  with runtime data injected.
- **Context files**: Crush reads AGENTS.md, CRUSH.md, CLAUDE.md, GEMINI.md
  (and `.local` variants) from the working directory for project-specific
  instructions.
- **Persistence**: SQLite + sqlc. All queries live in `internal/db/sql/`,
  generated code in `internal/db/`. Migrations in `internal/db/migrations/`.
- **Pub/sub**: `internal/pubsub` for decoupled communication between agent,
  UI, and services.
- **CGO disabled**: builds with `CGO_ENABLED=0` and
  `GOEXPERIMENT=greenteagc`.

## Build/Test/Lint Commands

- **Build**: `go build .` or `go run .`
- **Test**: `task test` or `go test ./...` (run single test:
  `go test ./internal/llm/prompt -run TestGetContextFromPaths`)
- **Update Golden Files**: `go test ./... -update` (regenerates `.golden`
  files when test output changes)
  - Update specific package:
    `go test ./internal/tui/components/core -update` (in this case,
    we're updating "core")
- **Lint**: `task lint:fix`
- **Format**: `task fmt` (`gofumpt -w .`)
- **Modernize**: `task modernize` (runs `modernize` which makes code
  simplifications)
- **Dev**: `task dev` (runs with profiling enabled)

## Code Style Guidelines

- **Imports**: Use `goimports` formatting, group stdlib, external, internal
  packages.
- **Formatting**: Use gofumpt (stricter than gofmt), enabled in
  golangci-lint.
- **Naming**: Standard Go conventions — PascalCase for exported, camelCase
  for unexported.
- **Types**: Prefer explicit types, use type aliases for clarity (e.g.,
  `type AgentName string`).
- **Error handling**: Return errors explicitly, use `fmt.Errorf` for
  wrapping.
- **Context**: Always pass `context.Context` as first parameter for
  operations.
- **Interfaces**: Define interfaces in consuming packages, keep them small
  and focused.
- **Structs**: Use struct embedding for composition, group related fields.
- **Constants**: Use typed constants with iota for enums, group in const
  blocks.
- **Testing**: Use testify's `require` package, parallel tests with
  `t.Parallel()`, `t.SetEnv()` to set environment variables. Always use
  `t.Tempdir()` when in need of a temporary directory. This directory does
  not need to be removed.
- **JSON tags**: Use snake_case for JSON field names.
- **File permissions**: Use octal notation (0o755, 0o644) for file
  permissions.
- **Log messages**: Log messages must start with a capital letter (e.g.,
  "Failed to save session" not "failed to save session").
  - This is enforced by `task lint:log` which runs as part of `task lint`.
- **Comments**: End comments in periods unless comments are at the end of the
  line.

## Testing with Mock Providers

When writing tests that involve provider configurations, use the mock
providers to avoid API calls:

```go
func TestYourFunction(t *testing.T) {
    // Enable mock providers for testing
    originalUseMock := config.UseMockProviders
    config.UseMockProviders = true
    defer func() {
        config.UseMockProviders = originalUseMock
        config.ResetProviders()
    }()

    // Reset providers to ensure fresh mock data
    config.ResetProviders()

    // Your test code here - providers will now return mock data
    providers := config.Providers()
    // ... test logic
}
```

## Formatting

- ALWAYS format any Go code you write.
  - First, try `gofumpt -w .`.
  - If `gofumpt` is not available, use `goimports`.
  - If `goimports` is not available, use `gofmt`.
  - You can also use `task fmt` to run `gofumpt -w .` on the entire project,
    as long as `gofumpt` is on the `PATH`.

## Comments

- Comments that live on their own lines should start with capital letters and
  end with periods. Wrap comments at 78 columns.

## Committing

- ALWAYS use semantic commits (`fix:`, `feat:`, `chore:`, `refactor:`,
  `docs:`, `sec:`, etc).
- Try to keep commits to one line, not including your attribution. Only use
  multi-line commits when additional context is truly necessary.

## Working on the TUI (UI)

Anytime you need to work on the TUI, read `internal/ui/AGENTS.md` before
starting work.
