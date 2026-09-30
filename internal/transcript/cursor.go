package transcript

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Cursor agent transcript: ~/.cursor/projects/<project>/agent-transcripts/<id>/<id>.jsonl
// (subagents under <id>/subagents/), one {"role", "message": {"content": [...]}}
// object per line. Cursor records tool calls as intended, with no timestamps,
// call ids, results or token usage.

type cursorLine struct {
	Role    string `json:"role"`
	Message struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
}

// cursorTools maps Cursor tool names (lowercased) to canonical ones. Cursor
// documents Shell, Read, Write and Delete; StrReplace and ApplyPatch come
// from observed transcripts, and the snake_case names from older versions.
var cursorTools = map[string]string{
	"read":             "Read",
	"read_file":        "Read",
	"write":            "Write",
	"edit_file":        "Edit",
	"strreplace":       "Edit",
	"search_replace":   "Edit",
	"delete":           "Delete",
	"delete_file":      "Delete",
	"shell":            "Bash",
	"run_terminal_cmd": "Bash",
}

// cursorFields maps Cursor input field names to canonical ones.
var cursorFields = map[string]string{
	"path":        "file_path",
	"target_file": "file_path",
	"contents":    "content",
	"fileText":    "content",
	"code_edit":   "new_string",
}

var userQuery = regexp.MustCompile(`(?s)<user_query>\s*(.*?)\s*</user_query>`)

func parseCursor(b []byte) []Event {
	var events []Event
	n := 0
	lines(b, func(raw []byte) bool {
		var l cursorLine
		if json.Unmarshal(raw, &l) != nil || l.Role == "" {
			return true
		}
		for _, c := range l.Message.Content {
			var e Event
			switch {
			case c.Type == "text" && l.Role == "user":
				t := c.Text
				if m := userQuery.FindStringSubmatch(t); m != nil {
					t = m[1]
				}
				e.Kind, e.Text = Prompt, strings.TrimSpace(t)
			case c.Type == "text":
				t := strings.TrimSpace(strings.ReplaceAll(c.Text, "[REDACTED]", ""))
				e.Kind, e.Text = Text, t
			case c.Type == "tool_use":
				n++
				e.Kind, e.ToolID = ToolUse, "cursor-"+strconv.Itoa(n)
				e.Tool = c.Name
				t, mapped := cursorTools[strings.ToLower(c.Name)]
				if mapped {
					e.Tool = t
				}
				var patch string
				if json.Unmarshal(c.Input, &patch) == nil && strings.EqualFold(c.Name, "ApplyPatch") {
					events = append(events, patchEvents(e, e.ToolID, patch)...)
					continue
				}
				fields := map[string]json.RawMessage{}
				_ = json.Unmarshal(c.Input, &fields)
				in := map[string]json.RawMessage{}
				for k, v := range fields {
					if f, ok := cursorFields[k]; ok && mapped {
						k = f
					}
					in[k] = v
				}
				var wd string
				if json.Unmarshal(in["working_directory"], &wd) == nil {
					e.Cwd = wd
				}
				e.Input, _ = json.Marshal(in)
			default:
				continue
			}
			if e.Kind != ToolUse && e.Text == "" {
				continue
			}
			events = append(events, e)
		}
		return true
	})
	return events
}
