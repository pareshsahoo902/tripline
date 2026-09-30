// Package transcript parses coding-agent session transcripts (Claude Code,
// Codex CLI, Cursor) into one flat event list.
//
// Tool calls use canonical names so rules and blame work across agents:
// Bash and PowerShell {command}, Read {file_path}, Edit {file_path,
// old_string, new_string}, Write {file_path, content}, Delete {file_path},
// plus Claude Code's MultiEdit and NotebookEdit. Other tools keep the name
// their agent gives them.
package transcript

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pareshsahoo902/tripline/internal/shell"
)

type Kind int

const (
	Prompt     Kind = iota // text typed by the user
	Text                   // assistant text or visible thinking
	ToolUse                // assistant requested a tool call
	ToolResult             // result of a tool call
)

type Usage struct {
	Input, Output, CacheWrite5m, CacheWrite1h, CacheRead int64
}

type Event struct {
	Kind      Kind
	Time      time.Time
	Session   string
	Model     string
	Cwd       string // working directory of the call, when the agent records it
	Text      string
	Tool      string
	ToolID    string
	Input     json.RawMessage
	IsError   bool
	Usage     *Usage // set on one event per API response
	Sidechain bool
}

// ParseFile parses the transcript at path. Events that carry no time or
// session id (Cursor records neither) get the file's modification time and
// base name.
func ParseFile(path string) ([]Event, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	events := Parse(b)
	var mod time.Time
	if info, err := os.Stat(path); err == nil {
		mod = info.ModTime()
	}
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for i := range events {
		if events[i].Time.IsZero() {
			events[i].Time = mod
		}
		if events[i].Session == "" {
			events[i].Session = id
		}
	}
	return events, nil
}

// Parse parses a transcript in any supported format, detected from its
// first line. Lines that fail to decode are skipped: a live transcript can
// end in a partially written line.
func Parse(b []byte) []Event {
	var probe struct {
		Role    string          `json:"role"`
		Payload json.RawMessage `json:"payload"`
	}
	lines(b, func(l []byte) bool {
		return json.Unmarshal(l, &probe) != nil // stop at the first valid line
	})
	switch {
	case probe.Payload != nil:
		return parseCodex(b)
	case probe.Role != "":
		return parseCursor(b)
	}
	return parseClaude(b)
}

// lines calls fn with each non-blank line until fn returns false.
func lines(b []byte, fn func([]byte) bool) {
	for len(b) > 0 {
		var l []byte
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			l, b = b[:i], b[i+1:]
		} else {
			l, b = b, nil
		}
		if l = bytes.TrimSpace(l); len(l) > 0 && !fn(l) {
			return
		}
	}
}

// Key identifies a tool call by tool name and canonical input.
// Re-marshaling through a map sorts object keys.
func (e Event) Key() string {
	var v any
	if json.Unmarshal(e.Input, &v) != nil {
		return e.Tool + ":" + string(e.Input)
	}
	b, _ := json.Marshal(v)
	return e.Tool + ":" + string(b)
}

// Field returns a string field of the tool input, or "".
func (e Event) Field(name string) string {
	var m map[string]any
	if json.Unmarshal(e.Input, &m) != nil {
		return ""
	}
	s, _ := m[name].(string)
	return s
}

// Writes returns the files a tool call modifies, joined onto the call's
// working directory when that is known. unknown is true when the call may
// modify files it doesn't name, such as a shell `git checkout` or `patch`.
// Shell commands are judged by shell.Writes, a heuristic.
func (e Event) Writes() (files []string, unknown bool) {
	if e.Kind != ToolUse {
		return nil, false
	}
	switch e.Tool {
	case "Edit", "MultiEdit", "Write", "Delete":
		files = []string{e.Field("file_path")}
	case "NotebookEdit":
		files = []string{e.Field("notebook_path")}
	case "Bash", "PowerShell":
		files, unknown = shell.Writes(e.Field("command"))
	default:
		return nil, false
	}
	out := files[:0]
	for _, f := range files {
		if f == "" {
			continue
		}
		if e.Cwd != "" && !filepath.IsAbs(f) && !strings.HasPrefix(f, "~") {
			f = filepath.Join(e.Cwd, f)
		}
		out = append(out, f)
	}
	return out, unknown
}

// Shell reports whether the call ran a shell command.
func (e Event) Shell() bool { return e.Tool == "Bash" || e.Tool == "PowerShell" }
