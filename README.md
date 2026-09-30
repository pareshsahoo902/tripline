# tripline

[![ci](https://github.com/pareshsahoo902/tripline/actions/workflows/ci.yml/badge.svg)](https://github.com/pareshsahoo902/tripline/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/pareshsahoo902/tripline)](https://github.com/pareshsahoo902/tripline/releases)
[![license](https://img.shields.io/github/license/pareshsahoo902/tripline)](LICENSE)

Guardrails and blame for AI coding agents. Local, single binary, no network.

Coding agents such as Claude Code run long tool-call loops while you're not looking. tripline sits in that loop:

- **`tripline hook`** stops a bad tool call *before* it runs. It catches the agent re-running the same failing command, re-reading a file it already has, blowing through a cost budget, or running something destructive like `rm -rf ~`, `git push --force` or `curl … | sh`. The agent sees why the call was blocked and changes course.
- **`tripline blame <file>`** answers "why did the agent change this?" For every agent edit to a file, it shows the prompt you gave, the agent's explanation right before the edit, and what changed.
- **`tripline scan`** replays a finished session: tool calls, errors, tokens, estimated cost, and every point where a rule would have fired.

Currently supports **Claude Code** on Windows, macOS and Linux.

```
$ tripline blame internal/auth/token.go
2026-09-30 17:20  Edit  session e59f0edf  ok
  prompt  token expiry is off by one, users get logged out early
  why     Expiry check uses < but needs <=.
  change  - if now < exp {
          + if now <= exp {
```

## Install

```sh
# macOS (Homebrew)
brew install --cask pareshsahoo902/tap/tripline

# Windows (Scoop)
scoop bucket add tripline https://github.com/pareshsahoo902/scoop-bucket
scoop install tripline

# Anywhere with Go 1.23+
go install github.com/pareshsahoo902/tripline@latest
```

Prebuilt binaries for Windows, macOS and Linux (amd64 and arm64) are on the [releases page](https://github.com/pareshsahoo902/tripline/releases). winget support is coming.

## Set up the hook

Add this to `~/.claude/settings.json` to guard every project, or to `.claude/settings.json` to guard one:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [{ "type": "command", "command": "tripline hook" }]
      }
    ]
  }
}
```

Restart Claude Code. When tripline blocks a call, Claude sees a message like this and adapts:

```
tripline blocked this call (loop): this exact Bash call already ran 3 times with no file changes in between.
It will give the same result. Change approach or ask the user.
```

## Rules

| Rule | Blocks when | Default | Flag |
|---|---|---|---|
| `loop` | The same tool call, with identical input, already ran N times and no file was edited in between | 3 | `--max-repeat N` |
| `reread` | `Read` of the same file with the same arguments already happened N times, and the file wasn't edited since | 2 | `--max-rereads N` |
| `budget` | Estimated session cost reached the limit | off | `--budget USD` |
| `risky` | A `Bash`/`PowerShell` command matches a destructive pattern (see [rules.go](internal/rules/rules.go)) | on | `--no-risky` |

Set a flag to `0` to turn a rule off. Flags go in the hook command, for example `"command": "tripline hook --budget 5 --max-repeat 4"`.

**Cost is an estimate.** It uses Anthropic API list prices. On a Pro or Max subscription, read it as "API-equivalent" usage, not money spent.

**tripline fails open.** If anything goes wrong inside tripline (unreadable transcript, unexpected input), it allows the call. A bug in tripline never stalls your agent.

## Commands

```
tripline hook   [--max-repeat N] [--max-rereads N] [--budget USD] [--no-risky]
tripline scan   [same flags] [--dir DIR] [transcript.jsonl]   # default: most recent session
tripline blame  [--dir DIR] [--json] <file>
tripline version
```

`--dir` defaults to `~/.claude/projects`, or `$CLAUDE_CONFIG_DIR/projects` when that variable is set.

`blame` searches every local session. It shows edits made through the `Edit`, `MultiEdit`, `Write` and `NotebookEdit` tools. Changes the agent made through shell commands (`sed -i`, `mv`, and so on) aren't attributed yet.

## How it works

Claude Code writes each session as JSONL under `~/.claude/projects/<project>/<session>.jsonl`. tripline parses that file on every invocation. It never modifies it, and it never sends anything over the network.

On each tool call, Claude Code runs `tripline hook` and passes it the session id, transcript path and pending tool call on stdin. tripline evaluates the rules against the session history. It exits `2` with a reason on stderr to block the call, or `0` to allow it.

## Roadmap

- Other agents: Codex CLI, Cursor, opencode
- `blame` for shell-command edits
- `tripline init` to install the hook for you
- Warn-only mode for rules

Issues and PRs welcome. See [docs/design.md](docs/design.md) for the design.

## License

MIT
