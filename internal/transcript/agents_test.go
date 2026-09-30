package transcript

import (
	"path/filepath"
	"testing"
)

// Shapes taken from a Codex 0.142 rollout file.
const codexSample = `{"timestamp":"2026-07-03T18:15:07.497Z","type":"session_meta","payload":{"session_id":"c1","id":"c1","cwd":"/repo","cli_version":"0.142.5"}}
{"timestamp":"2026-07-03T18:15:07.693Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"/repo","model":"gpt-5.5"}}
{"timestamp":"2026-07-03T18:15:07.691Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>x</environment_context>"},{"type":"input_text","text":"add a Dockerfile"}]}}
{"timestamp":"2026-07-03T18:15:07.829Z","type":"event_msg","payload":{"type":"user_message","message":"add a Dockerfile","images":[]}}
{"timestamp":"2026-07-03T18:15:17.275Z","type":"event_msg","payload":{"type":"agent_message","message":"Checking the layout first.","phase":"commentary"}}
{"timestamp":"2026-07-03T18:15:17.304Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"ls\",\"workdir\":\"/repo\"}","call_id":"call_1"}}
{"timestamp":"2026-07-03T18:15:19.655Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"Chunk ID: 1\nProcess exited with code 0\nOutput:\nmain.py\n"}}
{"timestamp":"2026-07-03T18:15:19.978Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":14475,"cached_input_tokens":2432,"output_tokens":216,"total_tokens":14691},"last_token_usage":{"input_tokens":14475,"cached_input_tokens":2432,"output_tokens":216,"total_tokens":14691}}}}
{"timestamp":"2026-07-03T18:15:19.990Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":14475,"cached_input_tokens":2432,"output_tokens":216,"total_tokens":14691},"last_token_usage":{"input_tokens":14475,"cached_input_tokens":2432,"output_tokens":216,"total_tokens":14691}}}}
{"timestamp":"2026-07-03T18:45:03.547Z","type":"response_item","payload":{"type":"custom_tool_call","status":"completed","call_id":"call_2","name":"apply_patch","input":"*** Begin Patch\n*** Add File: Dockerfile\n+FROM python:3.12-slim\n+CMD [\"python\"]\n*** Update File: app/main.py\n@@\n-print('hi')\n+print('hello')\n*** Delete File: old.txt\n*** End Patch"}}
{"timestamp":"2026-07-03T18:45:29.866Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_2","output":"Exit code: 0\nOutput:\nSuccess."}}
{"timestamp":"2026-07-03T18:46:00.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"pytest\"}","call_id":"call_3"}}
{"timestamp":"2026-07-03T18:46:05.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_3","output":"Process exited with code 1\nOutput:\nFAILED"}}
{"timestamp":"2026-07-03T18:47:00.000Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"sed -i s/a/b/ x.py\"],\"workdir\":\"/repo/sub\"}","call_id":"call_4"}}
`

func TestCodex(t *testing.T) {
	ev := Parse([]byte(codexSample))
	var prompts, texts []string
	var tools []string
	var usage []*Usage
	failed := map[string]bool{}
	for _, e := range ev {
		switch e.Kind {
		case Prompt:
			prompts = append(prompts, e.Text)
		case Text:
			if e.Usage != nil {
				usage = append(usage, e.Usage)
			} else {
				texts = append(texts, e.Text)
			}
		case ToolUse:
			tools = append(tools, e.Tool+" "+e.Field("file_path")+e.Field("command"))
			if e.Session != "c1" || e.Model != "gpt-5.5" {
				t.Errorf("session/model not carried: %+v", e)
			}
		case ToolResult:
			if e.IsError {
				failed[e.ToolID] = true
			}
		}
	}
	if len(prompts) != 1 || prompts[0] != "add a Dockerfile" {
		t.Errorf("prompts %q: the user_message event wins over the response_item copy", prompts)
	}
	if len(texts) != 1 {
		t.Errorf("texts %q", texts)
	}
	want := []string{
		"Bash ls",
		"Write " + filepath.Join("/repo", "Dockerfile"),
		"Edit " + filepath.Join("/repo", "app/main.py"),
		"Delete " + filepath.Join("/repo", "old.txt"),
		"Bash pytest",
		"Bash sed -i s/a/b/ x.py",
	}
	if len(tools) != len(want) {
		t.Fatalf("tools %q, want %q", tools, want)
	}
	for i := range want {
		if tools[i] != want[i] {
			t.Errorf("tool %d: %q, want %q", i, tools[i], want[i])
		}
	}
	if len(usage) != 1 || usage[0].Input != 14475-2432 || usage[0].CacheRead != 2432 || usage[0].Output != 216 {
		t.Errorf("usage %+v: repeated token_count must count once, cached split out", usage)
	}
	if !failed["call_3"] || failed["call_1"] || failed["call_2"] {
		t.Errorf("failed %v", failed)
	}
	for _, e := range ev {
		if e.Kind == ToolUse && e.ToolID == "call_4" {
			if files, _ := e.Writes(); len(files) != 1 || files[0] != filepath.Join("/repo/sub", "x.py") {
				t.Errorf("shell write in workdir: %q", files)
			}
		}
	}
	if _, priced := SessionCost(ev); priced {
		t.Error("gpt usage has no price")
	}
}

// Shapes taken from a Cursor agent-transcripts file.
const cursorSample = `{"role":"user","message":{"content":[{"type":"text","text":"<user_query>\nfix the build\n</user_query>"}]}}
{"role":"assistant","message":{"content":[{"type":"text","text":"Looking.\n\n[REDACTED]"},{"type":"tool_use","name":"Read","input":{"path":"C:\\repo\\main.go"}},{"type":"tool_use","name":"Grep","input":{"pattern":"x","-i":true}}]}}
{"type":"turn_ended","status":"success"}
{"role":"assistant","message":{"content":[{"type":"tool_use","name":"Shell","input":{"command":"sed -i s/a/b/ x.go","working_directory":"/repo"}},{"type":"tool_use","name":"StrReplace","input":{"path":"/repo/a.go","old_string":"x","new_string":"y"}},{"type":"tool_use","name":"ApplyPatch","input":"*** Begin Patch\n*** Add File: /repo/new.go\n+package main\n*** End Patch"}]}}
`

func TestCursor(t *testing.T) {
	ev := Parse([]byte(cursorSample))
	if len(ev) != 7 {
		t.Fatalf("got %d events: %+v", len(ev), ev)
	}
	if ev[0].Kind != Prompt || ev[0].Text != "fix the build" {
		t.Errorf("prompt %+v", ev[0])
	}
	if ev[1].Kind != Text || ev[1].Text != "Looking." {
		t.Errorf("text %+v", ev[1])
	}
	if ev[2].Tool != "Read" || ev[2].Field("file_path") != `C:\repo\main.go` || ev[2].ToolID == ev[3].ToolID {
		t.Errorf("read %+v", ev[2])
	}
	if ev[3].Tool != "Grep" || ev[3].Field("pattern") != "x" {
		t.Errorf("grep %+v", ev[3])
	}
	if files, _ := ev[4].Writes(); ev[4].Tool != "Bash" || len(files) != 1 || files[0] != filepath.Join("/repo", "x.go") {
		t.Errorf("shell %+v %q", ev[4], files)
	}
	if ev[5].Tool != "Edit" || ev[5].Field("file_path") != "/repo/a.go" || ev[5].Field("new_string") != "y" {
		t.Errorf("strreplace %+v", ev[5])
	}
	if ev[6].Tool != "Write" || ev[6].Field("file_path") != "/repo/new.go" {
		t.Errorf("applypatch %+v", ev[6])
	}
}

func TestWritesAndMatch(t *testing.T) {
	cwd, _ := filepath.Abs("repo")
	e := Event{Kind: ToolUse, Tool: "Bash", Cwd: cwd, Input: []byte(`{"command":"rm -rf build && git stash"}`)}
	files, unknown := e.Writes()
	if len(files) != 1 || files[0] != filepath.Join(cwd, "build") || !unknown {
		t.Errorf("got %q %v", files, unknown)
	}
	if !Match(files[0], filepath.Join(cwd, "build", "out.bin")) {
		t.Error("deleting a directory touches files inside it")
	}
	if Match(files[0], filepath.Join(cwd, "buildx")) {
		t.Error("prefix match must stop at a path separator")
	}
	if !Match(filepath.Join(cwd, "*.o"), filepath.Join(cwd, "a.o")) {
		t.Error("glob")
	}
	if files, unknown := (Event{Kind: ToolUse, Tool: "Bash", Input: []byte(`{"command":"go test ./..."}`)}).Writes(); files != nil || unknown {
		t.Errorf("read-only command: %q %v", files, unknown)
	}
}
