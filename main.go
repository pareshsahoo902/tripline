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
  tripline hook [flags]         Claude Code PreToolUse hook: blocks loops, rereads, overspend, risky commands
  tripline scan [flags] [file]  Replay rules over a session transcript (default: most recent) and summarize it
  tripline blame [flags] <file> Show every agent edit to <file>, with the prompt and explanation behind it
  tripline version

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
		os.Exit(runHook(args, os.Stdin, os.Stderr))
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
	if err != nil {
		fmt.Fprintln(os.Stderr, "tripline:", err)
		os.Exit(1)
	}
}

// ruleFlags registers rule flags on fl. Call the returned function after
// fl.Parse to get the config.
func ruleFlags(fl *flag.FlagSet) func() rules.Config {
	c := rules.Default
	fl.IntVar(&c.MaxRepeat, "max-repeat", c.MaxRepeat, "identical tool calls allowed with no file change in between (0 = off)")
	fl.IntVar(&c.MaxRereads, "max-rereads", c.MaxRereads, "identical reads allowed of an unchanged file (0 = off)")
	fl.Float64Var(&c.BudgetUSD, "budget", c.BudgetUSD, "estimated session cost limit in USD (0 = off)")
	noRisky := fl.Bool("no-risky", false, "allow destructive shell commands")
	return func() rules.Config {
		c.Risky = !*noRisky
		return c
	}
}

type hookInput struct {
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolUseID      string          `json:"tool_use_id"`
}

// runHook returns the process exit code. 2 blocks the tool call and shows
// stderr to the agent. Any internal failure returns 0: tripline must never
// stall the agent because of its own bug.
func runHook(args []string, stdin io.Reader, stderr io.Writer) int {
	fl := flag.NewFlagSet("hook", flag.ContinueOnError)
	cfg := ruleFlags(fl)
	if fl.Parse(args) != nil {
		return 0
	}
	c := cfg()

	var in hookInput
	if json.NewDecoder(stdin).Decode(&in) != nil || in.TranscriptPath == "" || in.ToolName == "" {
		return 0
	}
	events, err := transcript.ParseFile(in.TranscriptPath)
	if err != nil {
		return 0
	}
	next := transcript.Event{Kind: transcript.ToolUse, Tool: in.ToolName, ToolID: in.ToolUseID, Input: in.ToolInput, Session: in.SessionID}
	history := withoutPending(events, next)

	findings := rules.Check(history, next, c)
	if len(findings) == 0 {
		return 0
	}
	for _, f := range findings {
		fmt.Fprintf(stderr, "tripline blocked this call (%s): %s\n", f.Rule, f.Reason)
	}
	return 2
}

// withoutPending drops the pending call itself if Claude Code already wrote
// it to the transcript, so it is not counted as its own repeat.
func withoutPending(events []transcript.Event, next transcript.Event) []transcript.Event {
	done := map[string]bool{}
	for _, e := range events {
		if e.Kind == transcript.ToolResult {
			done[e.ToolID] = true
		}
	}
	key := next.Key()
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Kind != transcript.ToolUse || done[e.ToolID] {
			continue
		}
		if (next.ToolID != "" && e.ToolID == next.ToolID) || (next.ToolID == "" && e.Key() == key) {
			return append(events[:i:i], events[i+1:]...)
		}
	}
	return events
}

func runScan(args []string, w io.Writer) error {
	fl := flag.NewFlagSet("scan", flag.ContinueOnError)
	cfg := ruleFlags(fl)
	dir := fl.String("dir", projectsDir(), "Claude Code projects directory")
	if err := fl.Parse(args); err != nil {
		return err
	}
	c := cfg()

	path := fl.Arg(0)
	if path == "" {
		var err error
		if path, err = latestTranscript(*dir); err != nil {
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
				fmt.Fprintf(w, "  %s  %-7s %s %s\n    %s\n", e.Time.Local().Format("15:04:05"), f.Rule, e.Tool, clip(inputSummary(e), 80), f.Reason)
			}
		}
		if u := e.Usage; u != nil {
			tok.Input += u.Input
			tok.Output += u.Output
			tok.CacheRead += u.CacheRead
			tok.CacheWrite5m += u.CacheWrite5m + u.CacheWrite1h
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
	fmt.Fprintf(w, "est. cost    $%.2f (API list prices)\n", transcript.SessionCost(events))
	return nil
}

func runBlame(args []string, w io.Writer) error {
	fl := flag.NewFlagSet("blame", flag.ContinueOnError)
	dir := fl.String("dir", projectsDir(), "Claude Code projects directory")
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
	err := filepath.WalkDir(*dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Contains(bytes.ToLower(b), needle) {
			return nil
		}
		events, _ := transcript.Parse(bytes.NewReader(b))
		edits = append(edits, blame.Find(events, target)...)
		return nil
	})
	if err != nil {
		return err
	}
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
		fmt.Fprintf(w, "no agent edits to %s found in %s\n", target, *dir)
		return nil
	}
	for _, e := range edits {
		status := "ok"
		if e.Failed {
			status = "FAILED"
		}
		fmt.Fprintf(w, "%s  %s  session %s  %s\n", e.Time.Local().Format("2006-01-02 15:04"), e.Tool, short(e.Session), status)
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

func projectsDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

func latestTranscript(dir string) (string, error) {
	var best string
	var bestT time.Time
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".jsonl" {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(bestT) {
			best, bestT = p, info.ModTime()
		}
		return nil
	})
	if best == "" {
		return "", fmt.Errorf("no transcripts found in %s", dir)
	}
	return best, nil
}
