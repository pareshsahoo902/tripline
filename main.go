// Command tripline guards and explains AI coding agent sessions.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/pareshsahoo902/tripline/internal/blame"
	"github.com/pareshsahoo902/tripline/internal/rules"
	"github.com/pareshsahoo902/tripline/internal/transcript"
)

var version = "dev" // set by the release build

const usage = `tripline: guardrails and blame for AI coding agent sessions.

Usage:
  tripline init [flags]         Install the Claude Code hook (--remove to uninstall)
  tripline hook [flags]         Claude Code PreToolUse hook: flags loops, rereads, overspend, risky commands
  tripline scan [flags] [file]  Replay rules over a session transcript (default: most recent) and summarize it
  tripline blame [flags] <file> Show every agent edit to <file>, with the prompt and explanation behind it
  tripline version

Reads Claude Code, Codex CLI and Cursor transcripts.
Run "tripline <command> -h" for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "hook":
		os.Exit(runHook(args, os.Stdin, os.Stdout, os.Stderr))
	case "init":
		err = runInit(args, os.Stdout)
	case "scan":
		err = runScan(args, os.Stdout)
	case "blame":
		err = runBlame(args, os.Stdout)
	case "version", "--version", "-v":
		if bi, ok := debug.ReadBuildInfo(); ok && version == "dev" && bi.Main.Version != "(devel)" && bi.Main.Version != "" {
			version = strings.TrimPrefix(bi.Main.Version, "v") // go install builds skip ldflags
		}
		fmt.Println("tripline", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "tripline: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err == flag.ErrHelp {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tripline:", err)
		os.Exit(1)
	}
}

// ruleFlags registers rule flags on fl. Call the returned function after
// fl.Parse to get the config; its error reports unknown rule names.
func ruleFlags(fl *flag.FlagSet) func() (rules.Config, error) {
	c := rules.Default
	fl.IntVar(&c.MaxRepeat, "max-repeat", c.MaxRepeat, "identical tool calls allowed with no file change in between (0 = off)")
	fl.IntVar(&c.MaxRereads, "max-rereads", c.MaxRereads, "identical reads allowed of an unchanged file (0 = off)")
	fl.Float64Var(&c.BudgetUSD, "budget", c.BudgetUSD, "stop at every multiple of this estimated session cost in USD (0 = off)")
	noRisky := fl.Bool("no-risky", false, "allow destructive shell commands")
	ask := fl.String("ask", "", "comma-separated rules that ask the user instead of blocking, e.g. risky,budget")
	warn := fl.String("warn", "", "comma-separated rules that only warn the agent, e.g. loop,reread")
	return func() (rules.Config, error) {
		c.Risky = !*noRisky
		c.Actions = map[string]rules.Action{}
		err := rules.ParseActions(*ask, rules.Ask, c.Actions)
		if e := rules.ParseActions(*warn, rules.Warn, c.Actions); err == nil {
			err = e
		}
		return c, err
	}
}

func runScan(args []string, w io.Writer) error {
	fl := flag.NewFlagSet("scan", flag.ContinueOnError)
	cfg := ruleFlags(fl)
	dir := fl.String("dir", "", "transcript directory (default: Claude Code, Codex and Cursor locations)")
	if err := fl.Parse(args); err != nil {
		return err
	}
	c, err := cfg()
	if err != nil {
		return err
	}

	path := fl.Arg(0)
	if path == "" {
		if path, err = latestTranscript(sourceDirs(*dir)); err != nil {
			return err
		}
	}
	events, err := transcript.ParseFile(path)
	if err != nil {
		return err
	}

	var calls, errs, prompts int
	var tok transcript.Usage
	tools := map[string]int{}
	models := map[string]bool{}
	fmt.Fprintf(w, "%s\n\n", path)
	for i, e := range events {
		switch e.Kind {
		case transcript.Prompt:
			prompts++
		case transcript.ToolResult:
			if e.IsError {
				errs++
			}
		case transcript.ToolUse:
			calls++
			tools[e.Tool]++
			for _, f := range rules.Check(events[:i], e, c) {
				rule := f.Rule
				if f.Action != rules.Block {
					rule += " (" + f.Action.String() + ")"
				}
				fmt.Fprintf(w, "  %s  %-7s %s %s\n    %s\n", e.Time.Local().Format("15:04:05"), rule, e.Tool, clip(inputSummary(e), 80), f.Reason)
			}
		}
		if u := e.Usage; u != nil {
			tok.Input += u.Input
			tok.Output += u.Output
			tok.CacheRead += u.CacheRead
			tok.CacheWrite5m += u.CacheWrite5m + u.CacheWrite1h
			if e.Model != "" {
				models[e.Model] = true
			}
		}
	}

	type kv struct {
		k string
		v int
	}
	var top []kv
	for k, v := range tools {
		top = append(top, kv{k, v})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].v > top[j].v || top[i].v == top[j].v && top[i].k < top[j].k })
	var parts []string
	for i, t := range top {
		if i == 6 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", t.k, t.v))
	}

	fmt.Fprintf(w, "\nprompts      %d\n", prompts)
	fmt.Fprintf(w, "tool calls   %d (%d errors)  %s\n", calls, errs, strings.Join(parts, ", "))
	fmt.Fprintf(w, "tokens       in %d, out %d, cache write %d, cache read %d\n", tok.Input, tok.Output, tok.CacheWrite5m, tok.CacheRead)
	if cost, priced := transcript.SessionCost(events); priced {
		fmt.Fprintf(w, "est. cost    $%.2f (API list prices)\n", cost)
	} else {
		var names []string
		for m := range models {
			names = append(names, m)
		}
		sort.Strings(names)
		fmt.Fprintf(w, "est. cost    n/a (no prices for %s)\n", strings.Join(names, ", "))
	}
	return nil
}

func runBlame(args []string, w io.Writer) error {
	fl := flag.NewFlagSet("blame", flag.ContinueOnError)
	dir := fl.String("dir", "", "transcript directory (default: Claude Code, Codex and Cursor locations)")
	asJSON := fl.Bool("json", false, "print JSON")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if fl.NArg() != 1 || fl.Arg(0) == "" {
		return fmt.Errorf("blame needs exactly one file argument")
	}
	target := fl.Arg(0)
	needle := bytes.ToLower([]byte(filepath.Base(target)))

	var edits []blame.Edit
	walkTranscripts(sourceDirs(*dir), func(p string, _ fs.DirEntry) {
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Contains(bytes.ToLower(b), needle) {
			return
		}
		events, err := transcript.ParseFile(p) // for Cursor's file-based times and ids
		if err != nil {
			return
		}
		edits = append(edits, blame.Find(events, target)...)
	})
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Time.Before(edits[j].Time) })

	if *asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if edits == nil {
			edits = []blame.Edit{}
		}
		return enc.Encode(edits)
	}
	if len(edits) == 0 {
		fmt.Fprintf(w, "no agent edits to %s found\n", target)
		return nil
	}
	for _, e := range edits {
		status := "ok"
		if e.Failed {
			status = "FAILED"
		}
		tool := e.Tool
		if e.Inferred {
			tool += " (inferred)"
		}
		fmt.Fprintf(w, "%s  %s  session %s  %s\n", e.Time.Local().Format("2006-01-02 15:04"), tool, short(e.Session), status)
		fmt.Fprintf(w, "  prompt  %s\n", clip(e.Prompt, 160))
		if e.Why != "" {
			fmt.Fprintf(w, "  why     %s\n", clip(e.Why, 160))
		}
		for i, l := range strings.Split(e.Change, "\n") {
			label := "        "
			if i == 0 {
				label = "change  "
			}
			fmt.Fprintf(w, "  %s%s\n", label, clip(l, 150))
		}
		fmt.Fprintln(w)
	}
	return nil
}

func inputSummary(e transcript.Event) string {
	for _, k := range []string{"command", "file_path", "pattern", "url", "query"} {
		if s := e.Field(k); s != "" {
			return s
		}
	}
	return string(e.Input)
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// sourceDirs returns dir, or when it is empty the transcript directories of
// every supported agent.
func sourceDirs(dir string) []string {
	if dir != "" {
		return []string{dir}
	}
	home, _ := os.UserHomeDir()
	claude := filepath.Join(home, ".claude")
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		claude = d
	}
	codex := filepath.Join(home, ".codex")
	if d := os.Getenv("CODEX_HOME"); d != "" {
		codex = d
	}
	return []string{filepath.Join(claude, "projects"), filepath.Join(codex, "sessions"), filepath.Join(home, ".cursor", "projects")}
}

// walkTranscripts calls fn for each .jsonl transcript under dirs. Under
// Cursor's projects directory only agent-transcripts count.
func walkTranscripts(dirs []string, fn func(path string, d fs.DirEntry)) {
	for _, dir := range dirs {
		cursor := filepath.Base(filepath.Dir(dir)) == ".cursor"
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(p) != ".jsonl" {
				return nil
			}
			if cursor && !strings.Contains(filepath.ToSlash(p), "/agent-transcripts/") {
				return nil
			}
			fn(p, d)
			return nil
		})
	}
}

func latestTranscript(dirs []string) (string, error) {
	var best string
	var bestT time.Time
	walkTranscripts(dirs, func(p string, d fs.DirEntry) {
		if info, err := d.Info(); err == nil && info.ModTime().After(bestT) {
			best, bestT = p, info.ModTime()
		}
	})
	if best == "" {
		return "", fmt.Errorf("no transcripts found in %s", strings.Join(dirs, ", "))
	}
	return best, nil
}
