// Package shell guesses which files a shell command modifies.
//
// It is a heuristic over common Unix, git, formatter, package-manager and
// PowerShell commands. It can't see what scripts or programs do internally.
package shell

import (
	"path"
	"strings"
)

// Writes returns the files cmd likely modifies. Paths are as written in the
// command, joined onto any earlier "cd" in the same command; they may be
// relative and may contain glob patterns. unknown is true when part of the
// command modifies files it doesn't name (git checkout, patch < fix.diff).
// The command writes nothing when files is empty and unknown is false.
func Writes(cmd string) (files []string, unknown bool) {
	dir := ""
	for _, seg := range split(cmd) {
		words := unwrap(seg.words)
		if len(words) == 0 {
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(path.Base(strings.ReplaceAll(words[0], `\`, "/"))), ".exe")
		if name == "cd" || name == "pushd" || name == "set-location" || name == "sl" {
			if a := positional(words[1:]); len(a) > 0 {
				dir = join(dir, a[0])
			}
			continue
		}
		f, w := command(name, words[1:])
		for _, r := range seg.redirects {
			f, w = append(f, r), true
		}
		if !w {
			continue
		}
		if len(f) == 0 {
			unknown = true
		}
		for _, p := range f {
			files = append(files, join(dir, p))
		}
	}
	return files, unknown
}

func join(dir, p string) string {
	if dir == "" || isAbs(p) {
		return p
	}
	return strings.TrimSuffix(dir, "/") + "/" + p
}

func isAbs(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) || strings.HasPrefix(p, "~") ||
		len(p) >= 2 && p[1] == ':' // C:\ or C:/
}

var packageFiles = map[string][]string{
	"npm":  {"package.json", "package-lock.json"},
	"pnpm": {"package.json", "pnpm-lock.yaml"},
	"yarn": {"package.json", "yarn.lock"},
	"bun":  {"package.json", "bun.lock", "bun.lockb"},
}

// command returns the files a single command writes.
func command(name string, args []string) ([]string, bool) {
	pos := positional(args)
	switch name {
	case "bash", "sh", "zsh":
		if s := value(args, "-c", "-lc"); s != "" {
			return nested(s)
		}
	case "powershell", "pwsh":
		if s := value(args, "-Command", "-command", "-c"); s != "" {
			return nested(s)
		}
	case "cmd":
		if s := value(args, "/c", "/C"); s != "" {
			return nested(s)
		}
	case "find":
		return nil, has(args, "-delete")
	case "tee", "touch", "truncate", "rm", "unlink", "shred", "mv", "rmdir":
		return pos, true
	case "cp", "ln", "install":
		if t := value(args, "-t", "--target-directory"); t != "" {
			return []string{t}, true
		}
		if pos = positional(args, "-t", "--target-directory"); len(pos) > 0 {
			return pos[len(pos)-1:], true
		}
		return nil, true
	case "dd":
		for _, a := range args {
			if strings.HasPrefix(a, "of=") {
				return []string{a[3:]}, true
			}
		}
	case "sed":
		if !inPlace(args, false) {
			return nil, false
		}
		pos = positional(args, "-e", "--expression", "-f", "--file")
		if value(args, "-e", "--expression", "-f", "--file") != "" {
			return pos, true
		}
		return rest(pos), true
	case "perl":
		if inPlace(args, true) {
			return positional(args, "-e", "-E"), true
		}
	case "patch":
		return nil, true
	case "git":
		return git(args)
	case "gofmt", "goimports", "clang-format":
		if has(args, "-w", "-i") {
			return pos, true
		}
	case "go":
		switch sub(pos) {
		case "fmt", "generate":
			return nil, true
		case "get":
			return []string{"go.mod", "go.sum"}, true
		case "mod":
			if len(pos) > 1 && (pos[1] == "tidy" || pos[1] == "edit") {
				return []string{"go.mod", "go.sum"}, true
			}
		}
	case "prettier":
		if has(args, "--write", "-w") {
			return pos, true
		}
	case "eslint":
		if has(args, "--fix") {
			return pos, true
		}
	case "black", "isort", "autopep8", "rustfmt":
		if !has(args, "--check", "--diff") && (name != "autopep8" || has(args, "-i", "--in-place")) {
			return pos, true
		}
	case "ruff":
		s := sub(pos)
		if s == "format" && !has(args, "--check", "--diff") || s == "check" && has(args, "--fix") {
			return rest(pos), true
		}
	case "cargo":
		switch sub(pos) {
		case "fmt":
			return nil, true
		case "add", "remove", "rm", "update":
			return []string{"Cargo.toml", "Cargo.lock"}, true
		}
	case "terraform":
		if sub(pos) == "fmt" {
			return nil, true
		}
	case "npm", "pnpm", "yarn", "bun":
		switch sub(pos) {
		case "install", "i", "ci", "add", "remove", "rm", "uninstall", "un", "update", "up", "upgrade":
			return packageFiles[name], true
		case "":
			if name == "yarn" {
				return packageFiles[name], true
			}
		}
	case "set-content", "add-content", "out-file", "new-item", "ni", "remove-item", "ri", "del", "erase",
		"clear-content", "clc", "rename-item", "ren", "rni":
		return powershell(args, false), true
	case "move-item", "mi", "move":
		return powershell(args, true), true
	case "copy-item", "cpi", "copy":
		if d := psValue(args, "destination"); d != "" {
			return []string{d}, true
		}
		if f := powershell(args, true); len(f) > 1 {
			return f[len(f)-1:], true
		}
		return nil, true
	}
	return nil, false
}

// nested handles bash -c "...", powershell -Command "..." and cmd /c "...".
func nested(script string) ([]string, bool) {
	f, unknown := Writes(script)
	return f, unknown || len(f) > 0
}

func git(args []string) ([]string, bool) {
	// Skip global options such as -C dir and -c key=value.
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if args[0] == "-C" || args[0] == "-c" {
			args = args[1:]
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return nil, false
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "apply", "am", "merge", "pull", "rebase", "stash", "cherry-pick", "revert", "switch", "clean":
		return nil, true
	case "checkout", "restore":
		if i := index(args, "--"); i >= 0 {
			return args[i+1:], true
		}
		if sub == "restore" {
			return positional(args), true
		}
		return nil, true // branch switch or file checkout; can't tell which
	case "reset":
		return nil, has(args, "--hard", "--merge", "--keep")
	case "mv":
		return positional(args), true
	case "rm":
		return positional(args), !has(args, "--cached")
	}
	return nil, false
}

// powershell returns path arguments of a file cmdlet: -Path, -LiteralPath,
// -FilePath and -Destination values, then positional arguments (only the
// first unless all is set, as the second is usually -Value).
func powershell(args []string, all bool) []string {
	var named, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		p, v, inline := strings.Cut(strings.ToLower(a[1:]), ":")
		switch p {
		case "force", "recurse", "nonewline", "append", "confirm", "whatif", "passthru", "noclobber":
			continue // switches take no value
		}
		if !inline && i+1 < len(args) {
			i++
			v = args[i]
		} else if inline {
			v = a[len(p)+2:] // keep original case
		}
		switch p {
		case "path", "literalpath", "filepath", "destination", "lp", "pspath":
			named = append(named, v)
		}
	}
	if !all && len(pos) > 1 {
		pos = pos[:1]
	}
	return append(named, pos...)
}

// psValue returns the value of PowerShell parameter name (lowercase).
func psValue(args []string, name string) string {
	for i, a := range args {
		p, v, inline := strings.Cut(a, ":")
		if !strings.EqualFold(p, "-"+name) {
			continue
		}
		if inline {
			return v
		}
		if i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// inPlace reports whether sed (or perl, when perl is set) was given -i,
// alone or combined with other short flags (-ri, -pi, -i.bak).
func inPlace(args []string, perl bool) bool {
	for _, a := range args {
		if a == "--in-place" || strings.HasPrefix(a, "--in-place=") {
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			flags := a[1:]
			if perl {
				if i := strings.IndexByte(flags, 'e'); i >= 0 {
					flags = flags[:i] // -pie: -e's value follows
				}
			} else if strings.HasPrefix(flags, "e") || strings.HasPrefix(flags, "f") {
				continue
			}
			if strings.ContainsRune(flags, 'i') {
				return true
			}
		}
	}
	return false
}

// positional returns arguments that aren't options. valued lists options
// that take the following argument as their value.
func positional(args []string, valued ...string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return append(out, args[i+1:]...)
		case has([]string{a}, valued...):
			i++
		case strings.HasPrefix(a, "-") && a != "-":
		default:
			out = append(out, a)
		}
	}
	return out
}

func value(args []string, names ...string) string {
	for i, a := range args {
		for _, n := range names {
			if a == n && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(a, n+"=") {
				return a[len(n)+1:]
			}
		}
	}
	return ""
}

func has(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func index(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}

func sub(pos []string) string {
	if len(pos) == 0 {
		return ""
	}
	return pos[0]
}

func rest(pos []string) []string {
	if len(pos) == 0 {
		return nil
	}
	return pos[1:]
}

// unwrap strips leading VAR=value assignments and wrapper commands.
func unwrap(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		switch {
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "="):
			words = words[1:]
		case w == "{" || w == "}":
			words = words[1:]
		case w == "sudo" || w == "command" || w == "time" || w == "nohup" || w == "env" || w == "exec" || w == "xargs":
			words = words[1:]
			for len(words) > 0 && strings.HasPrefix(words[0], "-") { // sudo -u root, env -i
				if words[0] == "-u" || words[0] == "-g" {
					words = words[1:]
				}
				words = words[1:]
			}
		default:
			return words
		}
	}
	return words
}
