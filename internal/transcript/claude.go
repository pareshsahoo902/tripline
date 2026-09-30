package transcript

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Claude Code transcript: ~/.claude/projects/<project>/<session>.jsonl.

type claudeLine struct {
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"sessionId"`
	Cwd         string    `json:"cwd"`
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

type claudeBlock struct {
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

func parseClaude(b []byte) []Event {
	seen := map[string]bool{} // API message ids whose usage was counted
	var events []Event
	lines(b, func(l []byte) bool {
		events = appendClaude(events, l, seen)
		return true
	})
	return events
}

func appendClaude(events []Event, b []byte, seen map[string]bool) []Event {
	var l claudeLine
	if json.Unmarshal(b, &l) != nil || l.Message == nil || l.IsMeta {
		return events
	}
	base := Event{Time: l.Timestamp, Session: l.SessionID, Model: l.Message.Model, Cwd: l.Cwd, Sidechain: l.IsSidechain}

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
		var blocks []claudeBlock
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
		var blocks []claudeBlock
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
