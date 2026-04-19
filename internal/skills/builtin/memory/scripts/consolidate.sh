#!/usr/bin/env bash
# consolidate.sh — Extract session data from crush SQLite DB
#
# Usage: consolidate.sh [options]
#   --last                  Extract most recent session (default)
#   --session <id>          Extract specific session
#   --all-unconsolidated    Extract all sessions since last consolidation marker
#   --data-dir <path>       Path to .crush directory (default: .crush)
#   --max-chars <n>         Truncate output at N characters (default: 30000)
#   --verbose               Include tool result content (default: summaries only)
#
# Output: structured text suitable for agent review and memory consolidation.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DATA_DIR=".crush"
MODE="last"
SESSION_ID=""
MAX_CHARS=30000
VERBOSE="0"

while [[ $# -gt 0 ]]; do
    case $1 in
        --last) MODE="last"; shift ;;
        --session) MODE="session"; SESSION_ID="$2"; shift 2 ;;
        --all-unconsolidated) MODE="all"; shift ;;
        --data-dir) DATA_DIR="$2"; shift 2 ;;
        --max-chars) MAX_CHARS="$2"; shift 2 ;;
        --verbose) VERBOSE="1"; shift ;;
        -h|--help)
            sed -n '2,/^$/p' "$0" | sed 's/^# //; s/^#//'
            exit 0
            ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

DB="$DATA_DIR/crush.db"
MEMORY_DIR="$DATA_DIR/memory"
JQ_FILTER="$SCRIPT_DIR/format-messages.jq"

if [[ ! -f "$DB" ]]; then
    echo "ERROR: crush.db not found at $DB" >&2
    exit 1
fi

if [[ ! -f "$JQ_FILTER" ]]; then
    echo "ERROR: jq filter not found at $JQ_FILTER" >&2
    exit 1
fi

resolve_session() {
    case "$MODE" in
        last)
            sqlite3 "$DB" "SELECT id FROM sessions WHERE parent_session_id IS NULL ORDER BY updated_at DESC LIMIT 1"
            ;;
        session)
            echo "$SESSION_ID"
            ;;
        all)
            local marker=""
            if [[ -f "$MEMORY_DIR/last-consolidated" ]]; then
                marker=$(cat "$MEMORY_DIR/last-consolidated")
            fi
            if [[ -n "$marker" ]] && [[ "$marker" != "0" ]]; then
                sqlite3 "$DB" "SELECT id FROM sessions WHERE parent_session_id IS NULL AND updated_at > $marker ORDER BY updated_at ASC"
            else
                sqlite3 "$DB" "SELECT id FROM sessions WHERE parent_session_id IS NULL ORDER BY updated_at ASC"
            fi
            ;;
    esac
}

format_session() {
    local sid="$1"

    # Session header
    local title
    title=$(sqlite3 "$DB" "SELECT title FROM sessions WHERE id = '$sid'")
    local msg_count
    msg_count=$(sqlite3 "$DB" "SELECT message_count FROM sessions WHERE id = '$sid'")
    local tokens_in
    tokens_in=$(sqlite3 "$DB" "SELECT prompt_tokens FROM sessions WHERE id = '$sid'")
    local tokens_out
    tokens_out=$(sqlite3 "$DB" "SELECT completion_tokens FROM sessions WHERE id = '$sid'")
    local created
    created=$(sqlite3 "$DB" "SELECT datetime(created_at, 'unixepoch', 'localtime') FROM sessions WHERE id = '$sid'")

    echo "=== SESSION: \"$title\" ($created) ==="
    echo "  messages: $msg_count | tokens: ${tokens_in} in / ${tokens_out} out"
    echo ""

    # Extract and format messages
    sqlite3 -json "$DB" "
        SELECT role, parts
        FROM messages
        WHERE session_id = '$sid'
        ORDER BY created_at ASC
    " | jq -r -L "$SCRIPT_DIR" \
        --arg VERBOSE "$VERBOSE" \
        'include "format-messages"; format_messages' 2>/dev/null \
    || sqlite3 -json "$DB" "
        SELECT role, parts
        FROM messages
        WHERE session_id = '$sid'
        ORDER BY created_at ASC
    " | jq -r -f "$JQ_FILTER" --arg VERBOSE "$VERBOSE" 2>/dev/null
}

main() {
    if [[ "$MODE" == "all" ]]; then
        local sessions
        sessions=$(resolve_session)
        if [[ -z "$sessions" ]]; then
            echo "No unconsolidated sessions found."
            exit 0
        fi
        local total=0
        while IFS= read -r sid; do
            [[ -z "$sid" ]] && continue
            local result
            result=$(format_session "$sid")
            local len=${#result}
            if (( total + len > MAX_CHARS )); then
                echo "--- TRUNCATED: max chars ($MAX_CHARS) reached ---"
                break
            fi
            echo "$result"
            echo ""
            ((total += len))
        done <<< "$sessions"
    else
        local sid
        sid=$(resolve_session)
        if [[ -z "$sid" ]]; then
            echo "ERROR: No session found" >&2
            exit 1
        fi
        format_session "$sid"
    fi
}

main
