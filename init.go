package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pareshsahoo902/tripline/internal/install"
)

func runInit(args []string, w io.Writer) error {
	fl := flag.NewFlagSet("init", flag.ContinueOnError)
	cfg := ruleFlags(fl)
	agent := fl.String("agent", "claude", "agent to guard: claude (Claude Code) or codex (Codex CLI)")
	project := fl.Bool("project", false, "install for this project (.claude/settings.json or .codex/hooks.json) instead of for your user")
	remove := fl.Bool("remove", false, "remove the tripline hook")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fl.Arg(0))
	}
	if _, err := cfg(); err != nil {
		return err
	}
	path, err := hooksPath(*agent, *project)
	if err != nil {
		return err
	}

	if *remove {
		r, err := install.Remove(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "tripline hook %s: %s\n", r, path)
		return nil
	}
	command := hookCommand(fl)
	r, err := install.Add(path, command)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "tripline hook %s: %s\n  command: %s\n", r, path, command)
	switch {
	case r == install.Unchanged:
	case *agent == "codex":
		fmt.Fprintln(w, "Restart Codex to load it. Codex asks you to review and trust new hooks before they run.")
	default:
		fmt.Fprintln(w, "Restart Claude Code to load it.")
	}
	return nil
}

// hooksPath returns the file that holds the agent's hooks.
func hooksPath(agent string, project bool) (string, error) {
	dir, file, env := ".claude", "settings.json", "CLAUDE_CONFIG_DIR"
	switch agent {
	case "claude":
	case "codex":
		dir, file, env = ".codex", "hooks.json", "CODEX_HOME"
	default:
		return "", fmt.Errorf("unknown agent %q (agents: claude, codex)", agent)
	}
	if project {
		return filepath.Abs(filepath.Join(dir, file))
	}
	if d := os.Getenv(env); d != "" {
		return filepath.Join(d, file), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, dir, file), err
}

// hookCommand builds the hook command: this binary, then every rule flag
// the user set on init.
func hookCommand(fl *flag.FlagSet) string {
	parts := []string{executable(), "hook"}
	fl.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "project", "remove":
		case "agent":
			if f.Value.String() != "claude" {
				parts = append(parts, "--agent", f.Value.String())
			}
		case "no-risky":
			if f.Value.String() == "true" {
				parts = append(parts, "--no-risky")
			}
		default:
			parts = append(parts, "--"+f.Name, f.Value.String()) // values are numbers or validated rule names
		}
	})
	return strings.Join(parts, " ")
}

var osExecutable = os.Executable // replaced in tests

// executable returns "tripline" when that name on PATH is this binary, and
// otherwise this binary's absolute path with forward slashes, which Git Bash,
// PowerShell and sh all accept.
func executable() string {
	self, err := osExecutable()
	if err != nil {
		return "tripline"
	}
	if r, err := filepath.EvalSymlinks(self); err == nil {
		self = r
	}
	if p, err := exec.LookPath("tripline"); err == nil {
		if r, err := filepath.EvalSymlinks(p); err == nil && r == self {
			return "tripline"
		}
	}
	self = filepath.ToSlash(self)
	if strings.ContainsAny(self, "\"$`'") {
		return "tripline" // can't quote safely; rely on PATH
	}
	if strings.Contains(self, " ") {
		return `"` + self + `"`
	}
	return self
}
