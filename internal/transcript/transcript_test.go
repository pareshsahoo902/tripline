package transcript

import (
	"math"
	"strings"
	"testing"
)

// Two lines share one API message id, as Claude Code writes them; usage must count once.
const sample = `{"type":"user","sessionId":"s1","timestamp":"2026-09-30T10:00:00Z","message":{"role":"user","content":"fix the bug"}}
{"type":"user","sessionId":"s1","isMeta":true,"timestamp":"2026-09-30T10:00:00Z","message":{"role":"user","content":"injected"}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:01Z","message":{"id":"m1","model":"claude-sonnet-5-5","content":[{"type":"text","text":"Looking at main.go"}],"usage":{"input_tokens":1000000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:02Z","message":{"id":"m1","model":"claude-sonnet-5-5","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/p/main.go"}}],"usage":{"input_tokens":1000000,"output_tokens":0}}}
{"type":"user","sessionId":"s1","timestamp":"2026-09-30T10:00:03Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"nope"}]}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:04Z","message":{"id":"m2","model":"claude-sonnet-5-5","content":[{"type":"thinking","thinking":""}],"usage":{"input_tokens":0,"output_tokens":100000}}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:05Z","message":{"id":"m3","model":"claude-sonnet-5-5","conte`

func TestParse(t *testing.T) {
	ev := Parse([]byte(sample))
	kinds := []Kind{Prompt, Text, ToolUse, ToolResult, Text}
	if len(ev) != len(kinds) {
		t.Fatalf("got %d events, want %d: %+v", len(ev), len(kinds), ev)
	}
	for i, k := range kinds {
		if ev[i].Kind != k {
			t.Errorf("event %d kind %d, want %d", i, ev[i].Kind, k)
		}
	}
	if ev[2].Tool != "Read" || ev[2].Field("file_path") != "/p/main.go" || ev[2].Usage != nil {
		t.Errorf("bad tool use: %+v", ev[2])
	}
	if !ev[3].IsError || ev[3].ToolID != "t1" {
		t.Errorf("bad tool result: %+v", ev[3])
	}
	// 1M input at $2 + 100k output at $10 = $3; the duplicated m1 usage must not count.
	if got, priced := SessionCost(ev); math.Abs(got-3) > 1e-9 || !priced {
		t.Errorf("cost %v %v, want 3 true", got, priced)
	}
}

func TestCleanPrompt(t *testing.T) {
	in := "<command-message>x:y</command-message>\n<command-name>/x:y</command-name>\n<command-args>do it</command-args>\n<system-reminder>noise</system-reminder>"
	if got := strings.Join(strings.Fields(cleanPrompt(in)), " "); got != "/x:y do it" {
		t.Errorf("got %q", got)
	}
	if got := cleanPrompt("<system-reminder>only noise</system-reminder>"); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestKeyIgnoresFieldOrder(t *testing.T) {
	a := Event{Tool: "Bash", Input: []byte(`{"command":"ls","timeout":5}`)}
	b := Event{Tool: "Bash", Input: []byte(`{"timeout":5, "command":"ls"}`)}
	if a.Key() != b.Key() {
		t.Errorf("%s != %s", a.Key(), b.Key())
	}
}

func TestCostPrefix(t *testing.T) {
	u := Usage{Input: 1e6}
	for model, want := range map[string]float64{
		"claude-opus-5-5": 4, "claude-opus-5": 5, "claude-fable-5-1": 10, "claude-haiku-4-5-20251001": 1, "claude-new-model": 2,
	} {
		if got, ok := Cost(model, u); got != want || !ok {
			t.Errorf("%s: %v %v, want %v true", model, got, ok, want)
		}
	}
	if _, ok := Cost("gpt-5.5", u); ok {
		t.Error("non-Anthropic model must be unpriced")
	}
	if _, ok := Cost("gpt-5.5", Usage{}); !ok {
		t.Error("zero usage needs no price")
	}
}
