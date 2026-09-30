package transcript

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Codex CLI rollout: ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl, one
// {"timestamp","type","payload"} object per line.

type codexLine struct {
	Timestamp time.Time    `json:"timestamp"`
	Type      string       `json:"type"`
	Payload   codexPayload `json:"payload"`
}

type codexPayload struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`    // session_meta
	Cwd       string          `json:"cwd"`   // session_meta, turn_context
	Model     string          `json:"model"` // turn_context
	Role      string          `json:"role"`  // message
	Content   []codexContent  `json:"content"`
	Name      string          `json:"name"`      // function_call, custom_tool_call
	Arguments string          `json:"arguments"` // function_call: JSON text
	Input     string          `json:"input"`     // custom_tool_call: raw text
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`  // *_output: string, or {output, metadata}
	Message   string          `json:"message"` // user_message, agent_message
	Success   *bool           `json:"success"` // patch_apply_end
	Info      *struct {
		Total codexUsage `json:"total_token_usage"`
	} `json:"info"` // token_count
}

type codexContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexUsage struct {
	Input  int64 `json:"input_tokens"` // includes cached_input_tokens
	Cached int64 `json:"cached_input_tokens"`
	Output int64 `json:"output_tokens"`
	Total  int64 `json:"total_tokens"`
}

var codexExit = regexp.MustCompile(`(?:Process exited with code|Exit code:) (-?\d+)`)

func parseCodex(b []byte) []Event {
	var (
		events      []Event
		fallback    = map[int]bool{} // indexes of response_item prompt copies
		session     string
		cwd         string
		model       string
		prev        codexUsage // running totals at the last token_count
		eventPrompt bool       // prompts come from user_message events; response_item copies are a fallback
	)
	lines(b, func(raw []byte) bool {
		var l codexLine
		if json.Unmarshal(raw, &l) != nil {
			return true
		}
		p := l.Payload
		base := Event{Time: l.Timestamp, Session: session, Model: model, Cwd: cwd}
		switch l.Type + "/" + p.Type {
		case "session_meta/":
			session, cwd = p.ID, p.Cwd
		case "turn_context/":
			model = p.Model
			if p.Cwd != "" {
				cwd = p.Cwd
			}
		case "event_msg/user_message":
			eventPrompt = true
			if p.Message != "" {
				base.Kind, base.Text = Prompt, p.Message
				events = append(events, base)
			}
		case "event_msg/agent_message":
			if p.Message != "" {
				base.Kind, base.Text = Text, p.Message
				events = append(events, base)
			}
		case "event_msg/token_count":
			// total_token_usage is cumulative, and Codex re-emits token_count
			// when only rate limits change, so count the growth in totals.
			if p.Info != nil && p.Info.Total.Total > prev.Total {
				t := p.Info.Total
				in, cached := t.Input-prev.Input, t.Cached-prev.Cached
				base.Kind = Text
				base.Usage = &Usage{Input: in - cached, CacheRead: cached, Output: t.Output - prev.Output}
				events = append(events, base)
				prev = t
			}
		case "event_msg/patch_apply_end":
			if p.Success != nil && !*p.Success {
				base.Kind, base.ToolID, base.IsError = ToolResult, p.CallID, true
				events = append(events, base)
			}
		case "response_item/message":
			if p.Role == "user" {
				for _, c := range p.Content {
					if t := strings.TrimSpace(c.Text); t != "" && !strings.HasPrefix(t, "<") {
						e := base
						e.Kind, e.Text = Prompt, t
						fallback[len(events)] = true
						events = append(events, e)
					}
				}
			}
		case "response_item/function_call":
			events = append(events, codexCall(base, p.Name, p.CallID, p.Arguments)...)
		case "response_item/custom_tool_call":
			if p.Name == "apply_patch" {
				events = append(events, patchEvents(base, p.CallID, p.Input)...)
			} else {
				base.Kind, base.Tool, base.ToolID = ToolUse, p.Name, p.CallID
				base.Input, _ = json.Marshal(map[string]string{"input": p.Input})
				events = append(events, base)
			}
		case "response_item/function_call_output", "response_item/custom_tool_call_output":
			base.Kind, base.ToolID, base.IsError = ToolResult, p.CallID, codexFailed(p.Output)
			events = append(events, base)
		}
		return true
	})

	// Keep the response_item prompts only when the file has no user_message
	// events (older Codex versions).
	if !eventPrompt {
		return events
	}
	out := events[:0]
	for i, e := range events {
		if !fallback[i] {
			out = append(out, e)
		}
	}
	return out
}

// codexCall maps a function_call to canonical tool events.
func codexCall(base Event, name, id, args string) []Event {
	var a struct {
		Cmd     string          `json:"cmd"`     // exec_command
		Command json.RawMessage `json:"command"` // shell: argv array
		Workdir string          `json:"workdir"`
		Input   string          `json:"input"` // apply_patch as a function
	}
	_ = json.Unmarshal([]byte(args), &a)
	if a.Workdir != "" {
		base.Cwd = a.Workdir
	}
	base.Kind, base.ToolID = ToolUse, id

	switch name {
	case "apply_patch":
		return patchEvents(base, id, a.Input)
	case "exec_command", "shell", "container.exec", "local_shell":
		cmd := a.Cmd
		var argv []string
		if json.Unmarshal(a.Command, &argv) == nil && len(argv) > 0 {
			switch {
			case argv[0] == "apply_patch" && len(argv) > 1:
				return patchEvents(base, id, argv[1])
			case len(argv) >= 3 && (argv[1] == "-lc" || argv[1] == "-c" || strings.EqualFold(argv[1], "-Command")):
				cmd = argv[2] // ["bash", "-lc", "script"]
			default:
				cmd = strings.Join(argv, " ")
			}
		} else if s := ""; json.Unmarshal(a.Command, &s) == nil && s != "" {
			cmd = s
		}
		base.Tool = "Bash"
		base.Input, _ = json.Marshal(map[string]string{"command": cmd})
	default:
		base.Tool = name
		base.Input = json.RawMessage(args)
		if !json.Valid(base.Input) {
			base.Input, _ = json.Marshal(map[string]string{"arguments": args})
		}
	}
	return []Event{base}
}

// patchEvents turns an apply_patch body into one Write, Edit or Delete event
// per file. Patch paths are relative to the working directory.
func patchEvents(base Event, id, patch string) []Event {
	type file struct {
		op, path, moveTo string
		old, new         []string
	}
	var files []*file
	var cur *file
	for _, l := range strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "*** Add File: "):
			cur = &file{op: "add", path: l[len("*** Add File: "):]}
			files = append(files, cur)
		case strings.HasPrefix(l, "*** Update File: "):
			cur = &file{op: "update", path: l[len("*** Update File: "):]}
			files = append(files, cur)
		case strings.HasPrefix(l, "*** Delete File: "):
			cur = &file{op: "delete", path: l[len("*** Delete File: "):]}
			files = append(files, cur)
		case strings.HasPrefix(l, "*** Move to: ") && cur != nil:
			cur.moveTo = l[len("*** Move to: "):]
		case strings.HasPrefix(l, "***") || cur == nil:
		case strings.HasPrefix(l, "+"):
			cur.new = append(cur.new, l[1:])
		case strings.HasPrefix(l, "-"):
			cur.old = append(cur.old, l[1:])
		}
	}

	abs := func(p string) string {
		p = strings.TrimSpace(p)
		if base.Cwd != "" && !filepath.IsAbs(p) {
			return filepath.Join(base.Cwd, p)
		}
		return p
	}
	var out []Event
	emit := func(tool string, in map[string]string) {
		e := base
		e.Kind, e.Tool, e.ToolID = ToolUse, tool, id
		e.Input, _ = json.Marshal(in)
		out = append(out, e)
	}
	for _, f := range files {
		switch f.op {
		case "add":
			emit("Write", map[string]string{"file_path": abs(f.path), "content": strings.Join(f.new, "\n")})
		case "delete":
			emit("Delete", map[string]string{"file_path": abs(f.path)})
		case "update":
			p := f.path
			if f.moveTo != "" {
				emit("Delete", map[string]string{"file_path": abs(f.path)})
				p = f.moveTo
			}
			emit("Edit", map[string]string{"file_path": abs(p), "old_string": strings.Join(f.old, "\n"), "new_string": strings.Join(f.new, "\n")})
		}
	}
	return out
}

// codexFailed reports whether a tool output records a nonzero exit code.
func codexFailed(raw json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	var wrapped struct {
		Metadata struct {
			ExitCode *int `json:"exit_code"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(s), &wrapped) == nil && wrapped.Metadata.ExitCode != nil {
		return *wrapped.Metadata.ExitCode != 0
	}
	if m := codexExit.FindStringSubmatch(s); m != nil {
		code, _ := strconv.Atoi(m[1])
		return code != 0
	}
	return false
}
