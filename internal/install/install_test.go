package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// indent formats compact JSON the way the package writes it.
func indent(t *testing.T, s string) string {
	t.Helper()
	var b bytes.Buffer
	if err := json.Indent(&b, []byte(s), "", "  "); err != nil {
		t.Fatalf("bad test JSON %s: %v", s, err)
	}
	return b.String() + "\n"
}

func TestIsTripline(t *testing.T) {
	match := []string{
		"tripline hook",
		"tripline hook --budget 5",
		`"C:/Program Files/tripline/tripline.exe" hook`,
		"/opt/homebrew/bin/tripline hook -max-repeat 4",
		`C:\Users\me\go\bin\tripline.exe hook`,
		"TripLine.EXE hook",
		"'/usr/local/bin/tripline' hook",
		"  tripline\thook  ",
	}
	noMatch := []string{
		"", "tripline", "tripline scan", "echo tripline hook", "triplinex hook", "tripline hooks",
		"tripline HOOK", `"tripline hook"`, "/opt/tripline/bin hook", "tripline.exe.bak hook",
	}
	for _, c := range match {
		if !IsTripline(c) {
			t.Errorf("IsTripline(%q) = false, want true", c)
		}
	}
	for _, c := range noMatch {
		if IsTripline(c) {
			t.Errorf("IsTripline(%q) = true, want false", c)
		}
	}
}

func TestResultString(t *testing.T) {
	want := map[Result]string{Added: "added", Updated: "updated", Unchanged: "unchanged", Removed: "removed", NotFound: "not found"}
	for r, s := range want {
		if r.String() != s {
			t.Errorf("%d.String() = %q, want %q", int(r), r.String(), s)
		}
	}
}

func TestClaudeCreates(t *testing.T) {
	want := `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "tripline hook --budget 5"
          }
        ]
      }
    ]
  }
}
`
	tests := []struct {
		name    string
		exists  bool
		content string
	}{
		{"missing", false, ""},
		{"empty", true, ""},
		{"whitespace", true, " \n\t\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "home", ".claude", "settings.json")
			if tt.exists {
				mustWrite(t, path, tt.content)
			}
			res, err := Add(path, "tripline hook --budget 5")
			if err != nil || res != Added {
				t.Fatalf("got %v, %v; want added", res, err)
			}
			if got := mustRead(t, path); got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
			if got, want := exists(path+".bak"), tt.content != ""; got != want {
				t.Errorf(".bak exists = %v, want %v", got, want)
			}
			if !tt.exists && runtime.GOOS != "windows" {
				if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
					t.Errorf("new file: %v, %v; want mode 0644", info, err)
				}
			}
		})
	}
}

func TestClaudeKeepsOtherSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	in := `{
    "model": "opus",
    "permissions": {"allow": ["Bash(ls)"]},
    "hooks": {
        "PostToolUse": [
            {"matcher": "Edit|Write", "hooks": [{"type": "command", "command": "make fmt && echo \"<done>\""}]}
        ]
    }
}
`
	want := `{
  "model": "opus",
  "permissions": {
    "allow": [
      "Bash(ls)"
    ]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "make fmt && echo \"<done>\""
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "tripline hook"
          }
        ]
      }
    ]
  }
}
`
	mustWrite(t, path, in)
	mustWrite(t, path+".bak", "stale backup")
	res, err := Add(path, "tripline hook")
	if err != nil || res != Added {
		t.Fatalf("got %v, %v; want added", res, err)
	}
	if got := mustRead(t, path); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := mustRead(t, path+".bak"); got != in {
		t.Errorf(".bak = %q, want the original %q", got, in)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("dir has %d entries, want settings.json and settings.json.bak only", len(entries))
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name, in, cmd string
		want          Result
		out           string
	}{
		{
			"update flags, keep other fields",
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook","timeout":30}]}]}}`,
			"tripline hook --budget 5", Updated,
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook --budget 5","timeout":30}]}]}}`,
		},
		{
			"merge duplicates across groups",
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"/usr/local/bin/tripline hook"}]},{"matcher":"*","hooks":[{"type":"command","command":"tripline hook --budget 1"}]}]}}`,
			"tripline hook --budget 5", Updated,
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"tripline hook --budget 5"}]}]}}`,
		},
		{
			"duplicate of the same command",
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"}]},{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"}]}]}}`,
			"tripline hook", Updated,
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"}]}]}}`,
		},
		{
			"shared group",
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"./lint.sh"},{"type":"command","command":"tripline.exe hook"},{"type":"command","command":"tripline hook"}]}]}}`,
			"tripline hook --budget 5", Updated,
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"./lint.sh"},{"type":"command","command":"tripline hook --budget 5"}]}]}}`,
		},
		{
			"add next to other hooks",
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo tripline hook"}]}],"Stop":[]}}`,
			"tripline hook", Added,
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo tripline hook"}]},{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"}]}],"Stop":[]}}`,
		},
		{
			"add PreToolUse after other events",
			`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]},"model":"opus"}`,
			"tripline hook", Added,
			`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}],"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"}]}]},"model":"opus"}`,
		},
		{
			"malformed entries kept",
			`{"hooks":{"PreToolUse":["junk",{"matcher":"*"},{"hooks":"x"},{"hooks":[1,{"command":5},{"type":"command","command":"tripline hook"}]}]}}`,
			"tripline hook -max-repeat 4", Updated,
			`{"hooks":{"PreToolUse":["junk",{"matcher":"*"},{"hooks":"x"},{"hooks":[1,{"command":5},{"type":"command","command":"tripline hook -max-repeat 4"}]}]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			mustWrite(t, path, tt.in)
			res, err := Add(path, tt.cmd)
			if err != nil || res != tt.want {
				t.Fatalf("got %v, %v; want %v", res, err, tt.want)
			}
			if got, want := mustRead(t, path), indent(t, tt.out); got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
			if got := mustRead(t, path+".bak"); got != tt.in {
				t.Errorf(".bak = %q, want the original", got)
			}
		})
	}
}

func TestClaudeUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	in := `{ "hooks": {"PreToolUse": [ {"matcher": "*", "hooks": [{"type": "command", "command": "tripline hook --budget 5"}, {"type":"command","command":"./lint.sh"}]}]}}`
	mustWrite(t, path, in)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	res, err := Add(path, "tripline hook --budget 5")
	if err != nil || res != Unchanged {
		t.Fatalf("got %v, %v; want unchanged", res, err)
	}
	if got := mustRead(t, path); got != in {
		t.Errorf("file changed to %q", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Error(err)
	} else if !info.ModTime().Equal(old) {
		t.Errorf("mod time changed to %v", info.ModTime())
	}
	if exists(path + ".bak") {
		t.Error("unchanged run wrote a .bak")
	}
}

func TestClaudeNoHTMLEscape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	cmd := `"C:/Program Files/tripline/tripline.exe" hook --budget 5 && echo "<ok>"`
	if res, err := Add(path, cmd); err != nil || res != Added {
		t.Fatalf("got %v, %v; want added", res, err)
	}
	want := indent(t, `{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"\"C:/Program Files/tripline/tripline.exe\" hook --budget 5 && echo \"<ok>\""}]}]}}`)
	if got := mustRead(t, path); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if res, err := Add(path, cmd); err != nil || res != Unchanged {
		t.Errorf("second run: got %v, %v; want unchanged", res, err)
	}
}

func TestClaudeRejectsOtherCommands(t *testing.T) {
	for _, cmd := range []string{"", "echo hi", "tripline scan"} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if _, err := Add(path, cmd); err == nil {
			t.Errorf("Add(%q): no error", cmd)
		}
		if exists(path) {
			t.Errorf("Add(%q) created the file", cmd)
		}
	}
}

func TestRemove(t *testing.T) {
	tests := []struct{ name, in, out string }{
		{
			"only tripline",
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"}]}]}}`,
			`{}`,
		},
		{
			"other settings kept",
			`{"model":"opus","hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook --budget 5"}]}],"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]},"env":{"A":"1"}}`,
			`{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"say done"}]}]},"env":{"A":"1"}}`,
		},
		{
			"shared group",
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline hook"},{"type":"command","command":"./lint.sh"}]}]}}`,
			`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"./lint.sh"}]}]}}`,
		},
		{
			"every copy, other groups kept",
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"tripline hook"}]},{"matcher":"Edit","hooks":[]},{"matcher":"*","hooks":[{"type":"command","command":"\"C:/tools/tripline.exe\" hook"}]}]}}`,
			`{"hooks":{"PreToolUse":[{"matcher":"Edit","hooks":[]}]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			mustWrite(t, path, tt.in)
			res, err := Remove(path)
			if err != nil || res != Removed {
				t.Fatalf("got %v, %v; want removed", res, err)
			}
			if got, want := mustRead(t, path), indent(t, tt.out); got != want {
				t.Errorf("got\n%s\nwant\n%s", got, want)
			}
			if got := mustRead(t, path+".bak"); got != tt.in {
				t.Errorf(".bak = %q, want the original", got)
			}
		})
	}
}

func TestRemoveClaudeNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if res, err := Remove(path); err != nil || res != NotFound {
		t.Errorf("missing file: got %v, %v; want not found", res, err)
	}
	if exists(path) {
		t.Error("missing file was created")
	}
	for _, in := range []string{"", `{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"tripline scan"}]}]}}`} {
		mustWrite(t, path, in)
		if res, err := Remove(path); err != nil || res != NotFound {
			t.Errorf("%q: got %v, %v; want not found", in, res, err)
		}
		if got := mustRead(t, path); got != in {
			t.Errorf("%q: file changed to %q", in, got)
		}
		if exists(path + ".bak") {
			t.Errorf("%q: wrote a .bak", in)
		}
	}
}

func TestBadFilesUntouched(t *testing.T) {
	tests := []struct{ name, in, msg string }{
		{"invalid JSON", "{\n  \"model\": \"opus\",\n}\n", "invalid JSON at line 3"},
		{"truncated", `{"hooks": {`, "invalid JSON"},
		{"trailing data", `{} {}`, "invalid JSON"},
		{"top level array", `[]`, "top level is not a JSON object"},
		{"top level null", `null`, "top level is not a JSON object"},
		{"hooks array", `{"hooks":[]}`, `"hooks" is not a JSON object`},
		{"hooks null", `{"hooks":null}`, `"hooks" is not a JSON object`},
		{"PreToolUse object", `{"hooks":{"PreToolUse":{}}}`, `"hooks.PreToolUse" is not a JSON array`},
		{"PreToolUse string", `{"hooks":{"PreToolUse":"tripline hook"}}`, `"hooks.PreToolUse" is not a JSON array`},
	}
	ops := []struct {
		name string
		fn   func(string) (Result, error)
	}{
		{"Claude", func(p string) (Result, error) { return Add(p, "tripline hook") }},
		{"Remove", Remove},
	}
	for _, tt := range tests {
		for _, op := range ops {
			t.Run(tt.name+"/"+op.name, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "settings.json")
				mustWrite(t, path, tt.in)
				_, err := op.fn(path)
				if err == nil || !strings.Contains(err.Error(), tt.msg) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.msg)
				}
				if got := mustRead(t, path); got != tt.in {
					t.Errorf("file changed to %q", got)
				}
				if entries, _ := os.ReadDir(dir); len(entries) != 1 {
					t.Errorf("dir has %d entries, want only the settings file", len(entries))
				}
			})
		}
	}
}

func TestKeepsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permission bits on Windows")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	mustWrite(t, path, `{"env":{"TOKEN":"secret"}}`)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(path, "tripline hook"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + ".bak"} {
		if info, err := os.Stat(p); err != nil {
			t.Error(err)
		} else if info.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, want 0600", p, info.Mode().Perm())
		}
	}
}

func TestSymlinkKept(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	link := filepath.Join(dir, "settings.json")
	in := `{"model":"opus"}`
	mustWrite(t, target, in)
	if err := os.Symlink(target, link); err != nil {
		t.Skip("cannot create symlinks:", err)
	}
	if res, err := Add(link, "tripline hook"); err != nil || res != Added {
		t.Fatalf("got %v, %v; want added", res, err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink replaced by a regular file")
	}
	if got := mustRead(t, target); !strings.Contains(got, `"tripline hook"`) {
		t.Errorf("target not updated: %s", got)
	}
	if got := mustRead(t, link+".bak"); got != in {
		t.Errorf(".bak = %q, want the original", got)
	}
}

func TestBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("\xef\xbb\xbf{\"model\":\"opus\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, err := Add(path, "tripline hook"); err != nil || r != Added {
		t.Fatalf("got %v %v", r, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "{") || !strings.Contains(string(b), `"model": "opus"`) {
		t.Errorf("got %s", b)
	}
}
