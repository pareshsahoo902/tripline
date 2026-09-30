// Package rules decides whether a pending tool call should be blocked.
package rules

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/pareshsahoo902/tripline/internal/transcript"
)

// Action is what the hook does when a rule fires.
type Action int

const (
	Block Action = iota // stop the call; the agent sees the reason
	Ask                 // let the user decide; the user sees the reason
	Warn                // allow the call; the agent sees the reason
)

func (a Action) String() string { return [...]string{"block", "ask", "warn"}[a] }

// Names lists every rule.
var Names = []string{"loop", "reread", "budget", "risky"}

type Config struct {
	MaxRepeat  int               // loop: identical calls allowed with no file write in between; 0 = off
	MaxRereads int               // reread: identical reads allowed of an unchanged file; 0 = off
	BudgetUSD  float64           // budget: fire each time the estimated session cost passes a multiple of this; 0 = off
	Risky      bool              // risky: flag destructive shell commands
	Actions    map[string]Action // per rule; missing means Block
}

var Default = Config{MaxRepeat: 3, MaxRereads: 2, Risky: true}

type Finding struct {
	Rule   string
	Reason string
	Action Action
}

// Check evaluates next against the session history.
// history must not contain next itself.
func Check(history []transcript.Event, next transcript.Event, c Config) []Finding {
	var out []Finding
	add := func(rule, reason string) {
		out = append(out, Finding{rule, reason, c.Actions[rule]})
	}
	if rule, reason := checkRepeat(history, next, c); rule != "" {
		add(rule, reason)
	}
	if c.BudgetUSD > 0 {
		if now, crossed := checkpoint(history, c.BudgetUSD); crossed {
			add("budget", fmt.Sprintf(
				"estimated session cost passed $%.2f (now $%.2f at API list prices). Summarize progress and ask the user whether to continue. tripline checks again at $%.2f.",
				math.Floor(now/c.BudgetUSD)*c.BudgetUSD, now, (math.Floor(now/c.BudgetUSD)+1)*c.BudgetUSD))
		}
	}
	if c.Risky {
		if pat := riskyMatch(next); pat != "" {
			add("risky", fmt.Sprintf(
				"command matches destructive pattern %q. Ask the user to run it themselves if it is intended.", pat))
		}
	}
	return out
}

// checkRepeat counts earlier identical calls. For Read, a write to the same
// file resets the count; for any other tool, a write to any file resets it.
// Writes include shell commands that look like they modify files.
func checkRepeat(history []transcript.Event, next transcript.Event, c Config) (rule, reason string) {
	isRead := next.Tool == "Read"
	limit := c.MaxRepeat
	if isRead {
		limit = c.MaxRereads
	}
	if limit <= 0 {
		return "", ""
	}
	key, path := next.Key(), next.Field("file_path")
	n := 0
scan:
	for i := len(history) - 1; i >= 0; i-- {
		e := history[i]
		if e.Kind != transcript.ToolUse {
			continue
		}
		files, unknown := e.Writes()
		if unknown || len(files) > 0 && !isRead {
			break
		}
		for _, f := range files {
			if transcript.Match(f, path) {
				break scan
			}
		}
		if e.Key() == key {
			n++
		}
	}
	if n < limit {
		return "", ""
	}
	if isRead {
		return "reread", fmt.Sprintf(
			"%s was already read %d times with the same arguments and has not been changed since. Use the content you already have.",
			path, n)
	}
	return "loop", fmt.Sprintf(
		"this exact %s call already ran %d times with no file changes in between. It will give the same result. Change approach or ask the user.",
		next.Tool, n)
}

// checkpoint reports whether the estimated cost crossed a multiple of step
// since the last completed tool call, so each multiple fires once rather than
// blocking every call after the first. Parallel calls of one response may
// each fire. Sessions with unpriced models never fire.
func checkpoint(history []transcript.Event, step float64) (now float64, crossed bool) {
	done := map[string]bool{}
	for _, e := range history {
		if e.Kind == transcript.ToolResult {
			done[e.ToolID] = true
		}
	}
	last := -1 // index of the last tool call that has a result
	for i, e := range history {
		if e.Kind == transcript.ToolUse && done[e.ToolID] {
			last = i
		}
	}
	var before float64
	for i, e := range history {
		if e.Usage == nil {
			continue
		}
		c, ok := transcript.Cost(e.Model, *e.Usage)
		if !ok {
			return 0, false
		}
		now += c
		if i <= last {
			before += c
		}
	}
	return now, math.Floor(now/step) > math.Floor(before/step)
}

var risky = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]+\s+)*-[a-zA-Z]*[rR][a-zA-Z]*\s+(-[a-zA-Z]+\s+)*("?(/|~|\$HOME|\*|\.|\.\.)/?\*?"?)(\s|;|&|$)`),
	regexp.MustCompile(`\bgit\s+push\b.*(\s--force(\s|$)|\s-f(\s|$))`),
	regexp.MustCompile(`\bgit\s+reset\s+--hard\b`),
	regexp.MustCompile(`\bgit\s+clean\s+-[a-zA-Z]*f`),
	regexp.MustCompile(`\b(curl|wget|iwr|Invoke-WebRequest)\b[^|]*\|\s*(sudo\s+)?(ba|z)?sh\b`),
	regexp.MustCompile(`(?i)\b(iwr|irm|Invoke-WebRequest|Invoke-RestMethod)\b[^|]*\|\s*(iex|Invoke-Expression)\b`),
	regexp.MustCompile(`\bmkfs(\.\w+)?\b|\bdd\s+.*\bof=/dev/`),
	regexp.MustCompile(`\bchmod\s+-R\s+777\s+/`),
	regexp.MustCompile(`(?i)\bdrop\s+(table|database|schema)\b`),
	regexp.MustCompile(`(?i)\b(rd|rmdir)\s+/s\s+/q\s+[a-z]:\\?(\s|$)`),
	regexp.MustCompile(`(?i)\bformat\s+[a-z]:`),
	regexp.MustCompile(`(?i)\bRemove-Item\b.*-Recurse.*\s("?[a-z]:\\?"?|~|\$HOME|\$env:USERPROFILE)(\s|$)`),
}

func riskyMatch(e transcript.Event) string {
	if !e.Shell() {
		return ""
	}
	cmd := e.Field("command")
	for _, re := range risky {
		if m := re.FindString(cmd); m != "" {
			return strings.TrimSpace(m)
		}
	}
	return ""
}

// ParseActions sets action a for each rule in a comma-separated list such as
// "risky,budget". Unknown names are skipped and reported in the error.
func ParseActions(list string, a Action, into map[string]Action) error {
	var err error
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		known := false
		for _, n := range Names {
			known = known || n == name
		}
		if !known {
			if err == nil {
				err = fmt.Errorf("unknown rule %q (rules: %s)", name, strings.Join(Names, ", "))
			}
			continue
		}
		into[name] = a
	}
	return err
}
