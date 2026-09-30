package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pareshsahoo902/tripline/internal/rules"
	"github.com/pareshsahoo902/tripline/internal/transcript"
)

type hookInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	Cwd            string          `json:"cwd"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
}

// hookOutput is Claude Code's PreToolUse JSON response.
type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision,omitempty"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
		AdditionalContext        string `json:"additionalContext,omitempty"`
	} `json:"hookSpecificOutput"`
	SystemMessage string `json:"systemMessage,omitempty"`
}

// runHook returns the process exit code. Findings are handled by action:
// block exits 2 with the reasons on stderr, which Claude sees; ask returns
// permissionDecision "ask" so the user decides; warn allows the call and adds
// the reasons to Claude's context. Any internal failure returns 0: tripline
// must never stall the agent because of its own bug.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("hook", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	cfg := ruleFlags(fl)
	agent := fl.String("agent", "claude", "")
	if fl.Parse(args) != nil {
		return 0
	}
	c, _ := cfg() // unknown rule names in --ask/--warn are skipped

	raw, err := io.ReadAll(io.LimitReader(stdin, 16<<20))
	if err != nil {
		return 0
	}
	var in hookInput
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // some Windows hosts send a BOM
	if json.Unmarshal(raw, &in) != nil || in.TranscriptPath == "" || in.ToolName == "" {
		return 0
	}
	events, err := transcript.ParseFile(in.TranscriptPath)
	if err != nil {
		return 0
	}
	next := transcript.Event{Kind: transcript.ToolUse, Tool: in.ToolName, ToolID: in.ToolUseID, Input: in.ToolInput, Session: in.SessionID, Cwd: in.Cwd}
	findings := rules.Check(withoutPending(events, next), next, c)

	var block, ask, warn []string
	for _, f := range findings {
		if *agent == "codex" {
			// Codex treats "ask" and non-blocking output as hook failures and
			// runs the tool, so ask degrades to block and warn is dropped.
			if f.Action == rules.Warn {
				continue
			}
			f.Action = rules.Block
		}
		msg := fmt.Sprintf("tripline %s (%s): %s", verb[f.Action], f.Rule, f.Reason)
		switch f.Action {
		case rules.Ask:
			ask = append(ask, msg)
		case rules.Warn:
			warn = append(warn, msg)
		default:
			block = append(block, msg)
		}
	}
	if len(block) > 0 {
		fmt.Fprintln(stderr, strings.Join(block, "\n"))
		return 2
	}
	if len(ask)+len(warn) == 0 {
		return 0
	}
	var out hookOutput
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	if len(ask) > 0 {
		out.HookSpecificOutput.PermissionDecision = "ask"
		out.HookSpecificOutput.PermissionDecisionReason = strings.Join(ask, "\n")
	}
	if len(warn) > 0 {
		out.HookSpecificOutput.AdditionalContext = strings.Join(warn, "\n")
		out.SystemMessage = strings.Join(warn, "\n")
	}
	_ = json.NewEncoder(stdout).Encode(out)
	return 0
}

var verb = map[rules.Action]string{rules.Block: "blocked this call", rules.Ask: "flagged this call", rules.Warn: "warning"}

// withoutPending drops the pending call itself if the agent already wrote it
// to the transcript, so it is not counted as its own repeat. It matches by
// tool call id, falling back to the latest call with the same input that has
// no result yet. The call's token usage, which belongs to the whole
// response, is kept.
func withoutPending(events []transcript.Event, next transcript.Event) []transcript.Event {
	done := map[string]bool{}
	for _, e := range events {
		if e.Kind == transcript.ToolResult {
			done[e.ToolID] = true
		}
	}
	match := -1
	key := next.Key()
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Kind != transcript.ToolUse || done[e.ToolID] {
			continue
		}
		if next.ToolID != "" && e.ToolID == next.ToolID {
			match = i
			break
		}
		if match < 0 && e.Key() == key {
			match = i
		}
	}
	if match < 0 {
		return events
	}
	out := append([]transcript.Event(nil), events...)
	if u := out[match].Usage; u != nil {
		out[match] = transcript.Event{Kind: transcript.Text, Model: out[match].Model, Usage: u}
		return out
	}
	return append(out[:match], out[match+1:]...)
}
