# jq filter for formatting crush session messages
# Input: JSON array of {role, parts} from SQLite
# Output: formatted text lines for consolidation review

.[] |
.role as $role |
(.parts | fromjson? // .parts) as $parts |

if $role == "user" then
  ($parts | map(select(.type == "text") | .data.text) | first // "No text content")
  | "[USER] " + (.[0:200] | if length == 200 then . + "..." else . end)

elif $role == "assistant" then
  # Text content (truncated)
  ([$parts[] | select(.type == "text") | .data.text] | join("\n"))
  | if length > 0 then "[ASSISTANT] " + (.[0:400] | if length == 400 then . + "..." else . end) else empty end,

  # Tool calls (just names + truncated input)
  ([$parts[] | select(.type == "tool_call")]
   | map("[TOOL_CALL] " + .data.name + if (.data.input // null) then " (" + (.data.input[0:80] | tostring | if length == 80 then . + "..." else . end) + ")" else "" end)
   | .[]),

  # Finish reason
  ([$parts[] | select(.type == "finish") | .data.reason]
   | map("[FINISH] " + .) | .[])

elif $role == "tool" then
  ([$parts[] | select(.type == "tool_result")]
   | map(
       if .data.name then
         "[TOOL_RESULT] " + .data.name
         + if (.data.is_error // false) then " (ERROR)" else "" end
         + if (.data.content // null) then
             if ($VERBOSE == "1") then
               " → " + (.data.content[0:200] | if length == 200 then . + "..." else . end)
             else
               " → " + (.data.content | split("\n") | length | tostring) + " lines"
             end
           else " → (binary/no content)"
           end
       else empty
       end
   ) | .[])

else empty
end
