// Package transcript parses Claude Code session transcripts (JSONL) into a flat event list.
package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
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
	Text      string
	Tool      string
	ToolID    string
	Input     json.RawMessage
	IsError   bool
	Usage     *Usage // set on the first event of each API message only
	Sidechain bool
}

type rawLine struct {
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"sessionId"`
	IsMeta      bool      `json:"isMeta"`
	IsSidechain bool      `json:"isSidechain"`
	Message     *struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreation            *struct {
				Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
				Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

var (
	dropTags = regexp.MustCompile(`(?s)<(system-reminder|command-message|local-command-stdout|local-command-caveat)>.*?</(system-reminder|command-message|local-command-stdout|local-command-caveat)>`)
	keepTags = regexp.MustCompile(`(?s)<(command-name|command-args)>(.*?)</(command-name|command-args)>`)
)

// cleanPrompt strips the wrapper tags Claude Code adds around slash commands
// and injected context, keeping the command and its arguments.
func cleanPrompt(s string) string {
	s = dropTags.ReplaceAllString(s, "")
	s = keepTags.ReplaceAllString(s, "$2 ")
	return strings.TrimSpace(s)
}

// ParseFile parses the transcript at path.
func ParseFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads JSONL from r. Lines that fail to decode are skipped: a live
// transcript can end in a partially written line.
func Parse(r io.Reader) ([]Event, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	seen := map[string]bool{}
	var events []Event
	for {
		b, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(b)) > 0 {
			events = appendLine(events, b, seen)
		}
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return events, err
		}
	}
}

func appendLine(events []Event, b []byte, seen map[string]bool) []Event {
	var l rawLine
	if json.Unmarshal(b, &l) != nil || l.Message == nil || l.IsMeta {
		return events
	}
	base := Event{Time: l.Timestamp, Session: l.SessionID, Model: l.Message.Model, Sidechain: l.IsSidechain}

	switch l.Type {
	case "user":
		var s string
		if json.Unmarshal(l.Message.Content, &s) == nil {
			if s = cleanPrompt(s); s != "" {
				e := base
				e.Kind, e.Text = Prompt, s
				events = append(events, e)
			}
			return events
		}
		var blocks []rawBlock
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			return events
		}
		for _, bl := range blocks {
			e := base
			switch bl.Type {
			case "text":
				if e.Text = cleanPrompt(bl.Text); e.Text == "" {
					continue
				}
				e.Kind = Prompt
			case "tool_result":
				e.Kind, e.ToolID, e.IsError = ToolResult, bl.ToolUseID, bl.IsError
			default:
				continue
			}
			events = append(events, e)
		}

	case "assistant":
		var blocks []rawBlock
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			return events
		}
		var usage *Usage
		if u := l.Message.Usage; u != nil && !seen[l.Message.ID] {
			seen[l.Message.ID] = true
			usage = &Usage{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadInputTokens}
			if c := u.CacheCreation; c != nil && c.Ephemeral5m+c.Ephemeral1h > 0 {
				usage.CacheWrite5m, usage.CacheWrite1h = c.Ephemeral5m, c.Ephemeral1h
			} else {
				usage.CacheWrite5m = u.CacheCreationInputTokens
			}
		}
		for _, bl := range blocks {
			e := base
			switch bl.Type {
			case "text":
				e.Kind, e.Text = Text, bl.Text
			case "thinking":
				if bl.Thinking == "" {
					continue
				}
				e.Kind, e.Text = Text, bl.Thinking
			case "tool_use":
				e.Kind, e.Tool, e.ToolID, e.Input = ToolUse, bl.Name, bl.ID, bl.Input
			default:
				continue
			}
			e.Usage, usage = usage, nil
			events = append(events, e)
		}
		if usage != nil { // message had no kept blocks; keep its usage anyway
			e := base
			e.Kind, e.Usage = Text, usage
			events = append(events, e)
		}
	}
	return events
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

// WritePath returns the file a file-modifying tool call targets, or "".
func (e Event) WritePath() string {
	if e.Kind != ToolUse {
		return ""
	}
	switch e.Tool {
	case "Edit", "MultiEdit", "Write":
		return e.Field("file_path")
	case "NotebookEdit":
		return e.Field("notebook_path")
	}
	return ""
}
