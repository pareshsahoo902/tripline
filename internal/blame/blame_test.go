package blame

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pareshsahoo902/tripline/internal/transcript"
)

func TestFind(t *testing.T) {
	file, _ := filepath.Abs("auth.go")
	other, _ := filepath.Abs("other.go")
	// JSON-escape backslashes in Windows paths.
	j := func(p string) string { return strings.ReplaceAll(p, `\`, `\\`) }

	jsonl := `{"type":"user","sessionId":"s1","timestamp":"2026-09-30T10:00:00Z","message":{"content":"token expiry is off by one"}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:01Z","message":{"id":"m1","content":[{"type":"text","text":"Expiry check uses < but needs <=."}]}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:02Z","message":{"id":"m1","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"` + j(file) + `","old_string":"if now < exp {\nreturn","new_string":"if now <= exp {"}}]}}
{"type":"user","sessionId":"s1","timestamp":"2026-09-30T10:00:03Z","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:00:04Z","message":{"id":"m2","content":[{"type":"tool_use","id":"t2","name":"Write","input":{"file_path":"` + j(other) + `","content":"a\nb"}}]}}
{"type":"user","sessionId":"s1","timestamp":"2026-09-30T10:01:00Z","message":{"content":"now rewrite it"}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-09-30T10:01:01Z","message":{"id":"m3","content":[{"type":"tool_use","id":"t3","name":"Write","input":{"file_path":"` + j(strings.ToUpper(file[:1])+file[1:]) + `","content":"a\nb\nc"}}]}}
{"type":"user","sessionId":"s1","timestamp":"2026-09-30T10:01:02Z","message":{"content":[{"type":"tool_result","tool_use_id":"t3","is_error":true,"content":"denied"}]}}
`
	events, err := transcript.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatal(err)
	}
	got := Find(events, "auth.go")
	if len(got) != 2 {
		t.Fatalf("got %d edits, want 2: %+v", len(got), got)
	}
	e := got[0]
	if e.Prompt != "token expiry is off by one" || e.Why != "Expiry check uses < but needs <=." || e.Failed {
		t.Errorf("first edit: %+v", e)
	}
	if e.Change != "- if now < exp { …\n+ if now <= exp {" {
		t.Errorf("change: %q", e.Change)
	}
	e = got[1]
	if e.Prompt != "now rewrite it" || e.Why != "" || !e.Failed || e.Change != "wrote 3 lines" {
		t.Errorf("second edit: %+v", e)
	}
}
