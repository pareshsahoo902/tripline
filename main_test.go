package main

import (
	"bytes"
	"encoding/json"
	"flag"
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

	var stdout, stderr bytes.Buffer
	if code := runHook(nil, in("make", "t4"), &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "(loop)") {
		t.Errorf("4th identical call: code %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := runHook([]string{"-max-repeat", "5"}, in("make", "t4"), &stdout, &stderr); code != 0 {
		t.Errorf("raised limit: code %d, stderr %q", code, stderr.String())
	}
	if code := runHook(nil, in("make test", "t5"), &stdout, &stderr); code != 0 {
		t.Errorf("different command: code %d", code)
	}

	// warn: allowed, reason goes to Claude's context and the user.
	stdout.Reset()
	if code := runHook([]string{"--warn", "loop"}, in("make", "t4"), &stdout, &stderr); code != 0 {
		t.Errorf("warn: code %d", code)
	}
	var out hookOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || out.HookSpecificOutput.HookEventName != "PreToolUse" ||
		out.HookSpecificOutput.PermissionDecision != "" || !strings.Contains(out.HookSpecificOutput.AdditionalContext, "(loop)") || out.SystemMessage == "" {
		t.Errorf("warn output %q (%v)", stdout.String(), err)
	}

	// ask: the user decides; never "allow", which would skip permission prompts.
	stdout.Reset()
	if code := runHook([]string{"--ask", "risky"}, in("git push --force", "t6"), &stdout, &stderr); code != 0 {
		t.Errorf("ask: code %d", code)
	}
	out = hookOutput{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || out.HookSpecificOutput.PermissionDecision != "ask" ||
		!strings.Contains(out.HookSpecificOutput.PermissionDecisionReason, "(risky)") {
		t.Errorf("ask output %q (%v)", stdout.String(), err)
	}

	// block wins over ask and warn.
	stdout.Reset()
	if code := runHook([]string{"--ask", "risky"}, in("make", "t4"), &stdout, &stderr); code != 2 || stdout.Len() != 0 {
		t.Errorf("block with ask configured for another rule: code %d, stdout %q", code, stdout.String())
	}

	// A bad rule name in --ask must not disable the rules.
	if code := runHook([]string{"--ask", "nope"}, in("make", "t4"), &stdout, &stderr); code != 2 {
		t.Errorf("unknown rule name: code %d, want 2", code)
	}
}

func TestHookStripsBOM(t *testing.T) {
	tr := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(tr, nil, 0o644)
	in := "\xef\xbb\xbf" + `{"transcript_path":` + quote(tr) + `,"tool_name":"Bash","tool_input":{"command":"rm -rf ~"}}`
	var stdout, stderr bytes.Buffer
	if code := runHook(nil, strings.NewReader(in), &stdout, &stderr); code != 2 {
		t.Errorf("BOM-prefixed input must still be checked: code %d", code)
	}
}

func TestHookCodexDegradesAsk(t *testing.T) {
	tr := filepath.Join(t.TempDir(), "s.jsonl")
	os.WriteFile(tr, nil, 0o644)
	in := func() *strings.Reader {
		return strings.NewReader(`{"transcript_path":` + quote(tr) + `,"tool_name":"Bash","tool_input":{"command":"git push --force"}}`)
	}
	var stdout, stderr bytes.Buffer
	if code := runHook([]string{"--agent", "codex", "--ask", "risky"}, in(), &stdout, &stderr); code != 2 || stdout.Len() != 0 {
		t.Errorf("codex ask must block (Codex runs the tool on ask): code %d, stdout %q", code, stdout.String())
	}
	if code := runHook([]string{"--agent", "codex", "--warn", "risky"}, in(), &stdout, &stderr); code != 0 || stdout.Len() != 0 {
		t.Errorf("codex warn is a no-op: code %d, stdout %q", code, stdout.String())
	}
}

func TestInitCodex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	osExecutable = func() (string, error) { return filepath.Join(dir, "bin", "tripline"), nil }
	defer func() { osExecutable = os.Executable }()
	var out bytes.Buffer
	if err := runInit([]string{"--agent", "codex"}, &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "hooks.json"))
	if !strings.Contains(string(b), "hook --agent codex") || !strings.Contains(string(b), `"PreToolUse"`) {
		t.Errorf("hooks.json %s", b)
	}
	if err := runInit([]string{"--agent", "cursor"}, &out); err == nil {
		t.Error("unsupported agent must fail")
	}
}

func TestHookFailsOpen(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, stdin := range []string{"", "not json", `{"transcript_path":"/does/not/exist","tool_name":"Bash"}`} {
		if code := runHook(nil, strings.NewReader(stdin), &stdout, &stderr); code != 0 {
			t.Errorf("%q: code %d, want 0", stdin, code)
		}
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }

func TestHookCommand(t *testing.T) {
	fl := flag.NewFlagSet("init", flag.ContinueOnError)
	cfg := ruleFlags(fl)
	fl.Bool("project", false, "")
	if err := fl.Parse([]string{"--budget", "5", "--ask", "risky", "--no-risky=false", "--project"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg(); err != nil {
		t.Fatal(err)
	}
	cmd := hookCommand(fl)
	if !strings.HasSuffix(cmd, " hook --ask risky --budget 5") {
		t.Errorf("got %q", cmd)
	}
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	osExecutable = func() (string, error) { return filepath.Join(dir, "bin", "tripline"), nil }
	defer func() { osExecutable = os.Executable }()
	var out bytes.Buffer
	if err := runInit([]string{"--budget", "5"}, &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(b), "hook --budget 5") || !strings.Contains(out.String(), "added") {
		t.Errorf("settings %s, output %s", b, out.String())
	}
	if err := runInit([]string{"--ask", "bogus"}, &out); err == nil {
		t.Error("init must reject unknown rule names")
	}
	out.Reset()
	if err := runInit([]string{"--remove"}, &out); err != nil || !strings.Contains(out.String(), "removed") {
		t.Errorf("remove: %v %s", err, out.String())
	}
}
