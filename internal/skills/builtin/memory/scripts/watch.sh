#!/usr/bin/env bash
# watch.sh — Monitor crush.db for idle sessions, flag for consolidation
#
# Usage: watch.sh [options]
#   --data-dir <path>       Path to .crush directory (default: .crush)
#   --interval <seconds>    Polling interval (default: 60)
#   --idle-threshold <secs> Seconds of inactivity before flagging (default: 300)
#   --once                  Check once and exit (no background loop)
#   --stop                  Kill any running watch process
#
# Output: appends flagged session IDs to <data-dir>/memory/pending.jsonl

set -euo pipefail

DATA_DIR=".crush"
INTERVAL=60
IDLE_THRESHOLD=300
ONCE=0
PID_FILE=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --data-dir) DATA_DIR="$2"; shift 2 ;;
        --interval) INTERVAL="$2"; shift 2 ;;
        --idle-threshold) IDLE_THRESHOLD="$2"; shift 2 ;;
        --once) ONCE=1; shift ;;
        --stop)
            if [[ -f "$DATA_DIR/memory/.watch.pid" ]]; then
                kill "$(cat "$DATA_DIR/memory/.watch.pid")" 2>/dev/null || true
                rm -f "$DATA_DIR/memory/.watch.pid"
                echo "Watch stopped."
            fi
            exit 0
            ;;
        -h|--help)
            sed -n '2,/^$/p' "$0" | sed 's/^# //; s/^#//'
            exit 0
            ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

DB="$DATA_DIR/crush.db"
MEMORY_DIR="$DATA_DIR/memory"
PID_FILE="$MEMORY_DIR/.watch.pid"
PENDING="$MEMORY_DIR/pending.jsonl"

mkdir -p "$MEMORY_DIR"

if [[ ! -f "$DB" ]]; then
    echo "ERROR: crush.db not found at $DB" >&2
    exit 1
fi

# Get the last consolidation timestamp (seconds since epoch)
get_last_consolidated() {
    if [[ -f "$MEMORY_DIR/last-consolidated" ]]; then
        cat "$MEMORY_DIR/last-consolidated"
    else
        echo "0"
    fi
}

# Find sessions that are idle and not yet consolidated
check_sessions() {
    local last_cons
    last_cons=$(get_last_consolidated)
    local now
    now=$(date +%s)
    local cutoff=$(( now - IDLE_THRESHOLD ))

    sqlite3 "$DB" "
        SELECT id, title, updated_at
        FROM sessions
        WHERE parent_session_id IS NULL
          AND message_count > 0
          AND updated_at > $last_cons
          AND updated_at < $cutoff
        ORDER BY updated_at ASC
    " | while IFS='|' read -r sid title updated; do
        # Check if already in pending
        if [[ -f "$PENDING" ]] && grep -q "$sid" "$PENDING" 2>/dev/null; then
            continue
        fi
        echo "{\"session_id\": \"$sid\", \"title\": \"$title\", \"idle_since\": $updated, \"detected_at\": $now}" >> "$PENDING"
        echo "Flagged session: $title ($sid)"
    done
}

# Background loop or single check
if [[ "$ONCE" -eq 1 ]]; then
    check_sessions
    exit 0
fi

# Daemonize
echo $$ > "$PID_FILE"
trap 'rm -f "$PID_FILE"; exit 0' SIGTERM SIGINT

echo "Watching $DB (interval: ${INTERVAL}s, idle threshold: ${IDLE_THRESHOLD}s)"
echo "PID: $$ (stored in $PID_FILE)"

while true; do
    sleep "$INTERVAL"
    [[ -f "$DB" ]] || continue
    check_sessions
done
