// Package install adds the tripline hook to agent settings files and removes it.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Result says what Claude or Remove did.
type Result int

const (
	Added     Result = iota // new hook entry written
	Updated                 // existing tripline hook rewritten
	Unchanged               // hook already installed as asked; nothing written
	Removed                 // tripline hooks removed
	NotFound                // no tripline hook to remove; nothing written
)

func (r Result) String() string {
	switch r {
	case Added:
		return "added"
	case Updated:
		return "updated"
	case Unchanged:
		return "unchanged"
	case Removed:
		return "removed"
	case NotFound:
		return "not found"
	}
	return fmt.Sprintf("Result(%d)", int(r))
}

// Add installs command as a PreToolUse hook in the hooks file at path
// (Claude Code settings.json or Codex hooks.json, which share the format),
// or updates an existing tripline entry. Everything else in the file is kept.
func Add(path, command string) (Result, error) {
	if !IsTripline(command) {
		return 0, fmt.Errorf("%q does not run tripline hook", command)
	}
	s, err := load(path)
	if err != nil {
		return 0, err
	}
	pre, found := replace(s.pre, command)
	res := Updated
	switch {
	case len(found) == 0:
		pre, res = append(pre, newGroup(command)), Added
	case len(found) == 1 && found[0] == command:
		return Unchanged, nil
	}
	return res, s.save(pre)
}

// Remove removes every tripline hook from the settings file at path.
func Remove(path string) (Result, error) {
	s, err := load(path)
	if err != nil {
		return 0, err
	}
	pre, found := replace(s.pre, "")
	if len(found) == 0 {
		return NotFound, nil
	}
	return Removed, s.save(pre)
}

// IsTripline reports whether a hook command runs "tripline hook": its first
// word, unquoted, has base name tripline or tripline.exe in any case, and its
// second word is hook.
func IsTripline(command string) bool {
	exe, rest := firstWord(command)
	if args := strings.Fields(rest); len(args) == 0 || args[0] != "hook" {
		return false
	}
	exe = strings.ToLower(exe[strings.LastIndexAny(exe, `/\`)+1:])
	return exe == "tripline" || exe == "tripline.exe"
}

// firstWord splits s into its first word, without surrounding quotes, and
// the rest.
func firstWord(s string) (word, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s != "" && (s[0] == '"' || s[0] == '\'') {
		if i := strings.IndexByte(s[1:], s[0]); i >= 0 {
			return s[1 : i+1], s[i+2:]
		}
	}
	if i := strings.IndexFunc(s, unicode.IsSpace); i >= 0 {
		return s[:i], s[i:]
	}
	return s, ""
}

// settings is a Claude Code settings file, parsed just deep enough to edit
// hooks.PreToolUse.
type settings struct {
	path  string
	orig  []byte      // content as read; nil if the file did not exist
	perm  fs.FileMode // permission bits for the new content
	root  object
	hooks object            // root["hooks"]
	pre   []json.RawMessage // hooks["PreToolUse"]
}

// load reads the settings file at path. A missing or blank file loads as
// empty settings.
func load(path string) (*settings, error) {
	s := &settings{path: path, perm: 0o644}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if s.orig, err = os.ReadFile(path); err != nil {
		return nil, err
	}
	s.perm = info.Mode().Perm()
	data := bytes.TrimPrefix(s.orig, []byte("\xef\xbb\xbf")) // BOM written by PowerShell 5.1
	if len(bytes.Trim(data, " \t\r\n")) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, new(json.RawMessage)); err != nil {
		return nil, syntaxError(path, data, err)
	}
	var ok bool
	if s.root, ok = parseObject(data); !ok {
		return nil, fmt.Errorf("%s: top level is not a JSON object", path)
	}
	if raw, found := s.root.get("hooks"); found {
		if s.hooks, ok = parseObject(raw); !ok {
			return nil, fmt.Errorf(`%s: "hooks" is not a JSON object`, path)
		}
	}
	if raw, found := s.hooks.get("PreToolUse"); found {
		if raw[0] != '[' || json.Unmarshal(raw, &s.pre) != nil {
			return nil, fmt.Errorf(`%s: "hooks.PreToolUse" is not a JSON array`, path)
		}
	}
	return s, nil
}

// syntaxError describes invalid JSON in the file at path, with the line
// where parsing failed.
func syntaxError(path string, data []byte, err error) error {
	var se *json.SyntaxError
	if errors.As(err, &se) && se.Offset <= int64(len(data)) {
		line := 1 + bytes.Count(data[:se.Offset], []byte("\n"))
		return fmt.Errorf("%s: invalid JSON at line %d: %w", path, line, err)
	}
	return fmt.Errorf("%s: invalid JSON: %w", path, err)
}

// save stores pre as hooks.PreToolUse, deleting it and then hooks when they
// are empty, and writes the file.
func (s *settings) save(pre []json.RawMessage) error {
	if len(pre) > 0 {
		s.hooks.set("PreToolUse", array(pre))
	} else {
		s.hooks.del("PreToolUse")
	}
	if len(s.hooks) > 0 {
		s.root.set("hooks", s.hooks.raw())
	} else {
		s.root.del("hooks")
	}
	var b bytes.Buffer
	if err := json.Indent(&b, s.root.raw(), "", "  "); err != nil {
		return err
	}
	b.WriteByte('\n')
	return writeFile(s.path, s.orig, b.Bytes(), s.perm)
}

// replace sets the first tripline hook in the PreToolUse groups to command
// and deletes the others, or deletes them all if command is "". Groups it
// leaves with no hooks are dropped. Everything else is kept as is, including
// entries that are not shaped like hook groups. It returns the new groups
// and the commands of the tripline hooks it found.
func replace(groups []json.RawMessage, command string) ([]json.RawMessage, []string) {
	var out []json.RawMessage
	var found []string
	for _, g := range groups {
		group, _ := parseObject(g)
		var hooks []json.RawMessage
		if raw, ok := group.get("hooks"); ok && json.Unmarshal(raw, &hooks) != nil {
			hooks = nil // not an array: leave the group alone
		}
		n := len(found)
		var kept []json.RawMessage
		for _, h := range hooks {
			hook, _ := parseObject(h)
			cmd := hookCommand(hook)
			if !IsTripline(cmd) {
				kept = append(kept, h)
				continue
			}
			if len(found) == 0 && command != "" {
				hook.set("command", quote(command))
				kept = append(kept, hook.raw())
			}
			found = append(found, cmd)
		}
		switch {
		case len(found) == n:
			out = append(out, g)
		case len(kept) > 0:
			group.set("hooks", array(kept))
			out = append(out, group.raw())
		}
	}
	return out, found
}

// newGroup is the PreToolUse entry for a new tripline hook.
func newGroup(command string) json.RawMessage {
	return json.RawMessage(`{"matcher":"*","hooks":[{"type":"command","command":` + string(quote(command)) + `}]}`)
}

// hookCommand returns the command of a hook object, or "".
func hookCommand(hook object) string {
	raw, ok := hook.get("command")
	var cmd string
	if !ok || json.Unmarshal(raw, &cmd) != nil {
		return ""
	}
	return cmd
}

// object is a JSON object that keeps its key order. Values stay raw, so
// anything that is not edited keeps its exact content.
type object []member

type member struct {
	key string
	val json.RawMessage
}

// parseObject decodes data, which must be valid JSON, as an object. ok is
// false if data is not an object.
func parseObject(data []byte) (o object, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, false
		}
		o = append(o, member{tok.(string), val})
	}
	return o, true
}

// index returns the position of key, or -1. Like JSON.parse, which Claude
// Code uses, the last of duplicate keys wins.
func (o object) index(key string) int {
	for i := len(o) - 1; i >= 0; i-- {
		if o[i].key == key {
			return i
		}
	}
	return -1
}

func (o object) get(key string) (json.RawMessage, bool) {
	if i := o.index(key); i >= 0 {
		return o[i].val, true
	}
	return nil, false
}

// set replaces the value of key in place, or appends key.
func (o *object) set(key string, val json.RawMessage) {
	if i := o.index(key); i >= 0 {
		(*o)[i].val = val
		return
	}
	*o = append(*o, member{key, val})
}

// del removes key, including any duplicates.
func (o *object) del(key string) {
	kept := (*o)[:0]
	for _, m := range *o {
		if m.key != key {
			kept = append(kept, m)
		}
	}
	*o = kept
}

func (o object) raw() json.RawMessage {
	b := []byte{'{'}
	for i, m := range o {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, quote(m.key)...)
		b = append(b, ':')
		b = append(b, m.val...)
	}
	return append(b, '}')
}

func array(elems []json.RawMessage) json.RawMessage {
	b := []byte{'['}
	for i, e := range elems {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, e...)
	}
	return append(b, ']')
}

// quote encodes s as a JSON string. Unlike json.Marshal it leaves <, > and &
// alone, so a command like "a && b" stays readable.
func quote(s string) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s) // a string always encodes
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// writeFile replaces the file at path with data. A non-empty old version is
// copied to path.bak first. data goes to a temp file in the same directory
// that is then renamed over path, so the file is never half written.
func writeFile(path string, old, data []byte, perm fs.FileMode) error {
	if len(old) > 0 {
		if err := os.WriteFile(path+".bak", old, perm); err != nil {
			return err
		}
	}
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p // replace a symlinked file's target, not the link
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
