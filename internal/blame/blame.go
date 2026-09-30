// Package blame links file edits back to the prompt and explanation that caused them.
package blame

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/pareshsahoo902/tripline/internal/transcript"
)

type Edit struct {
	Time    time.Time `json:"time"`
	Session string    `json:"session"`
	Tool    string    `json:"tool"`
	Prompt  string    `json:"prompt"` // last user prompt before the edit
	Why     string    `json:"why"`    // last assistant text before the edit, within the same prompt
	Change  string    `json:"change"`
	Failed  bool      `json:"failed"`
}

// Find returns every edit to path in events, in order.
func Find(events []transcript.Event, path string) []Edit {
	want := Normalize(path)
	failed := map[string]bool{}
	for _, e := range events {
		if e.Kind == transcript.ToolResult && e.IsError {
			failed[e.ToolID] = true
		}
	}
	var out []Edit
	prompt, why := map[string]string{}, map[string]string{}
	for _, e := range events {
		switch e.Kind {
		case transcript.Prompt:
			prompt[e.Session], why[e.Session] = e.Text, ""
		case transcript.Text:
			if e.Text != "" {
				why[e.Session] = e.Text
			}
		case transcript.ToolUse:
			if p := e.WritePath(); p != "" && Normalize(p) == want {
				out = append(out, Edit{
					Time: e.Time, Session: e.Session, Tool: e.Tool,
					Prompt: prompt[e.Session], Why: why[e.Session],
					Change: change(e), Failed: failed[e.ToolID],
				})
			}
		}
	}
	return out
}

func change(e transcript.Event) string {
	switch e.Tool {
	case "Edit":
		return "- " + firstLine(e.Field("old_string")) + "\n+ " + firstLine(e.Field("new_string"))
	case "Write":
		return "wrote " + strconv.Itoa(strings.Count(e.Field("content"), "\n")+1) + " lines"
	case "MultiEdit":
		return "multiple edits"
	}
	return e.Tool
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

// Normalize makes paths comparable: absolute, forward slashes, and lowercase
// on Windows and macOS, whose default filesystems ignore case.
func Normalize(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	p = filepath.ToSlash(filepath.Clean(p))
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		p = strings.ToLower(p)
	}
	return p
}
