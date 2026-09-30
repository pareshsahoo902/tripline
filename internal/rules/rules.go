// Package rules decides whether a pending tool call should be blocked.
package rules

import (
	"fmt"
	"regexp"

	"github.com/pareshsahoo902/tripline/internal/transcript"
)

type Config struct {
	MaxRepeat  int     // loop: identical calls allowed with no file write in between; 0 = off
	MaxRereads int     // reread: identical reads allowed of an unchanged file; 0 = off
	BudgetUSD  float64 // budget: estimated session cost limit; 0 = off
	Risky      bool    // risky: block destructive shell commands
}

var Default = Config{MaxRepeat: 3, MaxRereads: 2, Risky: true}

type Finding struct {
	Rule   string
	Reason string
}

// Check evaluates next against the session history.
// history must not contain next itself.
func Check(history []transcript.Event, next transcript.Event, c Config) []Finding {
	var out []Finding
	if f, ok := checkRepeat(history, next, c); ok {
		out = append(out, f)
	}
	if c.BudgetUSD > 0 {
		if cost := transcript.SessionCost(history); cost >= c.BudgetUSD {
			out = append(out, Finding{"budget", fmt.Sprintf(
				"estimated session cost $%.2f reached the $%.2f budget. Stop and ask the user whether to continue.",
				cost, c.BudgetUSD)})
		}
	}
	if c.Risky {
		if pat := riskyMatch(next); pat != "" {
			out = append(out, Finding{"risky", fmt.Sprintf(
				"command matches destructive pattern %q. Ask the user to run it themselves if it is intended.", pat)})
		}
	}
	return out
}

// checkRepeat counts earlier identical calls. For Read, a write to the same
// file resets the count; for any other tool, a write to any file resets it.
func checkRepeat(history []transcript.Event, next transcript.Event, c Config) (Finding, bool) {
	isRead := next.Tool == "Read"
	limit := c.MaxRepeat
	if isRead {
		limit = c.MaxRereads
	}
	if limit <= 0 {
		return Finding{}, false
	}
	key, path := next.Key(), next.Field("file_path")
	n := 0
	for i := len(history) - 1; i >= 0; i-- {
		e := history[i]
		if e.Kind != transcript.ToolUse {
			continue
		}
		if w := e.WritePath(); w != "" && (!isRead || w == path) {
			break
		}
		if e.Key() == key {
			n++
		}
	}
	if n < limit {
		return Finding{}, false
	}
	if isRead {
		return Finding{"reread", fmt.Sprintf(
			"%s was already read %d times with the same arguments and has not been edited since. Use the content you already have.",
			path, n)}, true
	}
	return Finding{"loop", fmt.Sprintf(
		"this exact %s call already ran %d times with no file changes in between. It will give the same result. Change approach or ask the user.",
		next.Tool, n)}, true
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
	if e.Tool != "Bash" && e.Tool != "PowerShell" {
		return ""
	}
	cmd := e.Field("command")
	for _, re := range risky {
		if m := re.FindString(cmd); m != "" {
			return m
		}
	}
	return ""
}
