# Changelog

## v0.2.0 — 2026-10-01

### Added

- **`tripline init`** installs the hook for you. It covers Claude Code (`~/.claude/settings.json`, or `--project`) and Codex CLI (`--agent codex`, `~/.codex/hooks.json`). The rest of the file is preserved, the previous version is kept as `.bak`, and the write is atomic. Running it again updates the entry. `--remove` uninstalls.
- **Codex CLI support.** `tripline hook --agent codex` guards Codex 0.124+ through its PreToolUse hook. `scan` and `blame` read Codex sessions, including `apply_patch` edits.
- **Cursor support in `scan` and `blame`**, read from `~/.cursor/projects/*/agent-transcripts`. Cursor records no timestamps or results, so times come from the file.
- **Per-rule actions.** `--ask RULES` lets you decide through the agent's permission prompt. `--warn RULES` lets the call run and tells the agent why it looks wrong.
- **Shell-edit detection.** Commands that write files (`sed -i`, `>` redirects, `tee`, `mv`, `git checkout`, formatters, package managers, PowerShell `Set-Content`, …) now show up in `blame` as `(inferred)`, and reset the `loop` and `reread` rules.

### Changed

- `budget` fires once at each multiple of the budget ($5, $10, …). Previously it blocked every call after the first time the budget was reached, so the session couldn't continue.
- `scan` shows cost as `n/a` for models without known prices (such as GPT models), instead of pricing them as Sonnet.
- `scan` without a file picks the most recent session across all supported agents.

### Fixed

- `loop` no longer fires when the agent edits files through shell commands between test runs.
- The hook strips a UTF-8 byte order mark from its input. Some Windows hosts send one, and it made the hook fail open silently.
- The pending call's token usage is no longer dropped when estimating the budget.
- `go install` builds report their real version.

### Not yet

- Cursor hook: Cursor's response semantics for allow and ask couldn't be verified. Guessing wrong would either auto-approve commands or block everything.
- opencode: it stores sessions in SQLite since v1.2, which needs a new dependency or its CLI.

## v0.1.0 — 2026-09-30

First release: `hook`, `scan` and `blame` for Claude Code.
