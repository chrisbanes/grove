#!/usr/bin/env bash
# SessionStart hook for grove plugin
# Detects Grove context (workspace or golden copy) and injects it into conversations.

set -euo pipefail

# Escape a string for safe embedding in a JSON string value.
escape_for_json() {
    local s="$1"
    s="${s//\\/\\\\}"
    s="${s//\"/\\\"}"
    s="${s//$'\n'/\\n}"
    s="${s//$'\r'/\\r}"
    s="${s//$'\t'/\\t}"
    printf '%s' "$s"
}

# Walk up from $PWD looking for .grove/workspace.json or .grove/config.json.
find_grove_dir() {
    local dir="$PWD"
    while true; do
        if [ -f "${dir}/.grove/workspace.json" ]; then
            echo "workspace:${dir}"
            return 0
        fi
        if [ -f "${dir}/.grove/config.json" ]; then
            echo "golden:${dir}"
            return 0
        fi
        local parent
        parent="$(dirname "$dir")"
        if [ "$parent" = "$dir" ]; then
            # Reached filesystem root without finding anything.
            echo "none:"
            return 0
        fi
        dir="$parent"
    done
}

grove_result="$(find_grove_dir)"
grove_type="${grove_result%%:*}"
grove_dir="${grove_result#*:}"

# Determine what context message to build.
context_message=""

if [ "$grove_type" = "workspace" ]; then
    # Read fields from workspace.json using basic shell parsing via python3 or jq if available.
    workspace_json="${grove_dir}/.grove/workspace.json"

    ws_id=""
    ws_golden=""
    ws_branch=""

    if command -v jq >/dev/null 2>&1; then
        ws_id="$(jq -r '.id? // ""' "$workspace_json" 2>/dev/null || true)"
        ws_golden="$(jq -r '.golden_copy? // ""' "$workspace_json" 2>/dev/null || true)"
        ws_branch="$(jq -r '.branch? // ""' "$workspace_json" 2>/dev/null || true)"
    elif command -v python3 >/dev/null 2>&1; then
        ws_id="$(python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(d.get('id',''))" "$workspace_json" 2>/dev/null || true)"
        ws_golden="$(python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(d.get('golden_copy',''))" "$workspace_json" 2>/dev/null || true)"
        ws_branch="$(python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(d.get('branch',''))" "$workspace_json" 2>/dev/null || true)"
    fi

    context_message="You are in a Grove workspace (ID: ${ws_id:-unknown}). Golden copy: ${ws_golden:-unknown}. Branch: ${ws_branch:-unknown}. Use grove:finishing-grove-workspace when done."

elif [ "$grove_type" = "golden" ]; then
    context_message="This project uses Grove. Use grove:using-grove to create isolated workspaces."

else
    # No .grove/ found anywhere — silent exit.
    exit 0
fi

# Surface a CLI-not-installed warning when grove is absent but .grove/ exists.
if ! command -v grove >/dev/null 2>&1; then
    context_message="${context_message} Grove CLI is not installed. Use grove:grove-init for setup guidance."
fi

context_escaped="$(escape_for_json "$context_message")"

cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "<IMPORTANT>\n${context_escaped}\n</IMPORTANT>"
  }
}
EOF

exit 0
