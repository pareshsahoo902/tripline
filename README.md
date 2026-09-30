# tripline

[![ci](https://github.com/pareshsahoo902/tripline/actions/workflows/ci.yml/badge.svg)](https://github.com/pareshsahoo902/tripline/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/pareshsahoo902/tripline)](https://github.com/pareshsahoo902/tripline/releases)
[![license](https://img.shields.io/github/license/pareshsahoo902/tripline)](LICENSE)

Guardrails and blame for AI coding agents. Local, single binary, no network.

Coding agents such as Claude Code and Codex run long tool-call loops while you're not looking. tripline sits in that loop:

- **`tripline hook`** stops a bad tool call *before* it runs. It catches the agent re-running the same failing command, re-reading a file it already has, passing a cost budget, or running something destructive like `rm -rf ~`, `git push --force` or `curl … | sh`. Each rule can block the call, ask you, or just warn the agent.
- **`tripline blame <file>`** answers "why did the agent change this?" For every agent edit to a file, including edits made through shell commands like `sed -i`, it shows the prompt you gave, the agent's explanation right before the edit, and what changed.
- **`tripline scan`** replays a finished session: tool calls, errors, tokens, estimated cost, and every point where a rule would have fired.

| | Claude Code | Codex CLI | Cursor |
|---|---|---|---|
| `hook` (guard) | yes | yes (0.124+) | not yet |
| `scan`, `blame` | yes | yes | yes |

Windows, macOS and Linux.

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

Prebuilt binaries for Windows, macOS and Linux (amd64 and arm64) are on the [releases page](https://github.com/pareshsahoo902/tripline/releases).

## Set up the hook

```sh
tripline init                   # Claude Code, all projects (~/.claude/settings.json)
tripline init --agent codex     # Codex CLI (~/.codex/hooks.json)
tripline init --project         # this project only
tripline init --budget 5 --ask risky   # with rule flags
tripline init --remove          # uninstall
```

`init` adds one `PreToolUse` entry and leaves the rest of the file untouched. It keeps a `.bak` copy of the previous version. Running it again updates the entry instead of adding a second one. Restart the agent afterwards. Codex asks you to trust new hooks before they run.

When tripline blocks a call, the agent sees a message like this and adapts:

```
tripline blocked this call (loop): this exact Bash call already ran 3 times with no file changes in between.
It will give the same result. Change approach or ask the user.
```

Prefer editing JSON yourself? See [Hook setup](https://github.com/pareshsahoo902/tripline/wiki/Hook-Setup) in the wiki.

## Rules

| Rule | Fires when | Default | Flag |
|---|---|---|---|
| `loop` | The same tool call, with identical input, already ran N times and no file changed in between | 3 | `--max-repeat N` |
| `reread` | `Read` of the same file with the same arguments already happened N times, and the file hasn't changed since | 2 | `--max-rereads N` |
| `budget` | Estimated session cost passes a multiple of USD ($5, $10, $15, …) | off | `--budget USD` |
| `risky` | A shell command matches a destructive pattern (see [rules.go](internal/rules/rules.go)) | on | `--no-risky` |

Set a number to `0` to turn a rule off. "Changed" includes edits through shell commands such as `sed -i`, `>` redirects, `git checkout` or formatters.

**Actions.** By default every rule blocks. `--ask RULES` hands the decision to you instead, through the agent's permission prompt. `--warn RULES` lets the call run and tells the agent why it looks wrong. Both take a comma-separated list, for example `--ask risky --warn loop,reread`. Codex can't ask or warn from a hook, so with `--agent codex`, `ask` blocks and `warn` does nothing.

**Cost is an estimate.** It uses Anthropic API list prices. On a Pro or Max subscription, read it as "API-equivalent" usage, not money spent. tripline has no prices for other providers' models (such as Codex's GPT models), so budget doesn't apply there and `scan` shows the cost as n/a.

**tripline fails open.** If anything goes wrong inside tripline (unreadable transcript, unexpected input), it allows the call. A bug in tripline never stalls your agent.

## Commands

```
tripline init   [--agent claude|codex] [--project] [--remove] [rule flags]
tripline hook   [--agent claude|codex] [rule flags]
tripline scan   [rule flags] [--dir DIR] [transcript.jsonl]   # default: most recent session
tripline blame  [--dir DIR] [--json] <file>
tripline version

rule flags: --max-repeat N  --max-rereads N  --budget USD  --no-risky  --ask RULES  --warn RULES
```

`scan` and `blame` look in `~/.claude/projects`, `~/.codex/sessions` and `~/.cursor/projects` (honoring `CLAUDE_CONFIG_DIR` and `CODEX_HOME`), or only in `--dir`.

Shell-command edits in `blame` are marked `(inferred)`. They come from a heuristic that recognizes common commands, redirects, git operations, formatters and package managers. It can't see what scripts do internally.

## How it works

Each agent writes its session as JSONL on your disk. tripline parses that file on every invocation. It never modifies transcripts, and it never sends anything over the network.

On each tool call, the agent runs `tripline hook` and passes it the session id, transcript path and pending tool call on stdin. tripline evaluates the rules against the session history. It exits `2` with a reason on stderr to block the call. To ask or warn, it prints a JSON response. Otherwise it exits `0` to allow the call.

## Roadmap

- Cursor hook (read-only support is in)
- opencode (its sessions live in SQLite since v1.2)
- winget
- A pre-commit check that runs `blame` over staged files

Full documentation is in the [wiki](https://github.com/pareshsahoo902/tripline/wiki). Release notes are in [CHANGELOG.md](CHANGELOG.md). Issues and PRs are welcome; see [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/design.md](docs/design.md).

## License

MIT
