package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHook(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "s.jsonl")
	line := `{"type":"assistant","sessionId":"s","timestamp":"2026-09-30T10:00:00Z","message":{"id":"m%d","content":[{"type":"tool_use","id":"t%d","name":"Bash","input":{"command":"make"}}]}}` + "\n"
	res := `{"type":"user","sessionId":"s","timestamp":"2026-09-30T10:00:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"t%d","content":"fail"}]}}` + "\n"
	var b strings.Builder
	for i := 1; i <= 3; i++ {
		b.WriteString(strings.NewReplacer("%d", string(rune('0'+i))).Replace(line))
		b.WriteString(strings.NewReplacer("%d", string(rune('0'+i))).Replace(res))
	}
	// The pending 4th call is already in the transcript; it must not count itself.
	b.WriteString(strings.NewReplacer("%d", "4").Replace(line))
	if err := os.WriteFile(tr, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	in := func(cmd, id string) *strings.Reader {
		return strings.NewReader(`{"session_id":"s","transcript_path":` + quote(tr) + `,"tool_name":"Bash","tool_use_id":"` + id + `","tool_input":{"command":"` + cmd + `"}}`)
	}

	var stderr bytes.Buffer
	if code := runHook(nil, in("make", "t4"), &stderr); code != 2 || !strings.Contains(stderr.String(), "(loop)") {
		t.Errorf("4th identical call: code %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := runHook([]string{"-max-repeat", "5"}, in("make", "t4"), &stderr); code != 0 {
		t.Errorf("raised limit: code %d, stderr %q", code, stderr.String())
	}
	if code := runHook(nil, in("make test", "t5"), &stderr); code != 0 {
		t.Errorf("different command: code %d", code)
	}
}

func TestHookFailsOpen(t *testing.T) {
	var stderr bytes.Buffer
	for _, stdin := range []string{"", "not json", `{"transcript_path":"/does/not/exist","tool_name":"Bash"}`} {
		if code := runHook(nil, strings.NewReader(stdin), &stderr); code != 0 {
			t.Errorf("%q: code %d, want 0", stdin, code)
		}
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }
