# tripline — design

Status: v0.1 scope, 2026-09-30.

## Problem

AI coding agents (Claude Code first) run long tool-call loops unattended. Users only see the final diff. Three failures recur:

- **Waste:** the agent re-runs the same command or re-reads the same unchanged file over and over.
- **Runaway spend:** a session burns far more tokens than intended before anyone notices.
- **Danger:** a single bad shell command (`rm -rf ~`, `git push --force`, `curl | sh`).

After the fact, nobody can answer "why did the agent change this file?" without scrolling a multi-megabyte transcript.

Existing open-source tools are read-only viewers or cost counters. None of them acts *before* a tool call runs, and none answers the per-file "why" question.

## Goals (v0.1)

1. `tripline hook` — Claude Code `PreToolUse` hook. It reads the session transcript, evaluates rules, and blocks the pending tool call (exit code 2, reason on stderr, which Claude sees) when a rule fires.
2. `tripline scan [transcript]` — replays the rules over a finished session and prints a summary: tool calls, errors, tokens, estimated cost, and where each rule would have fired. Use it to tune thresholds and to audit sessions.
3. `tripline blame <file>` — lists every agent edit to a file across all local sessions. For each edit it shows the user prompt that led to it, the agent's explanation right before the edit, a change summary, and whether the edit succeeded.

Non-goals for v0.1: GUI, other agents (Codex, Cursor), config files, killing processes, network calls of any kind.

## Rules

| Rule | Fires when | Default |
|---|---|---|
| `loop` | Identical tool call (same tool + same input) already made N times with no file write in between | N = 3 |
| `reread` | `Read` of the same file with the same arguments already made N times, no write to that file since | N = 2 |
| `budget` | Estimated session cost (API list prices) ≥ limit | off |
| `risky` | `Bash`/`PowerShell` command matches a destructive pattern | on |

Every threshold is a flag on the hook command, and 0 disables a rule.

## Safety principle: fail open

The hook runs before every tool call. If tripline has a bug, it must never stall the user's agent. On any internal error (unreadable stdin, missing transcript, parse failure), it exits 0 and allows the call.

## Architecture

```
main.go                      CLI: hook | scan | blame | version
internal/transcript          JSONL → []Event (prompt, text, tool_use, tool_result, usage); price table
internal/rules               Check(history, next, config) → []Finding
internal/blame               Find(events, path) → []Edit
```

All three commands share one parser. Transcript lines that fail to parse are skipped, because the file may be mid-write during a live session.

Cost is an estimate. Usage is deduplicated per API message id, since Claude Code writes one line per content block and repeats the same usage on each. Prices come from a built-in table keyed by model-id prefix. Subscription users don't pay per token, so for them the figure is "API-equivalent".

## Distribution

- Single static Go binary: Windows, macOS, Linux (amd64 + arm64).
- `go install`, GitHub Releases, Homebrew tap, winget, Scoop — all via GoReleaser on a git tag.

## Testing

Unit tests per package against small synthetic JSONL fixtures. CI runs `go test` on Windows, macOS and Linux.
