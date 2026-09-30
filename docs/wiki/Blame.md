# Blame

```sh
tripline blame [--dir DIR] [--json] <file>
```

For every agent edit to `<file>`, across all your local Claude Code, Codex and Cursor sessions, `blame` shows:

- **prompt**: the last thing you typed before the edit
- **why**: the agent's last message or visible reasoning before the edit, within that same prompt
- **change**: the first line removed and added (`Edit`), or the line count written (`Write`)
- **status**: `ok`, or `FAILED` if the tool call returned an error

```
$ tripline blame internal/auth/token.go
2026-09-30 17:20  Edit  session e59f0edf  ok
  prompt  token expiry is off by one, users get logged out early
  why     Expiry check uses < but needs <=.
  change  - if now < exp {
          + if now <= exp {
```

Edits are listed oldest first.

## Paths

`<file>` can be relative or absolute. Relative paths resolve against the current directory, so run `blame` from the project the agent worked in. On Windows and macOS, matching ignores case.

## JSON output

`--json` prints an array of `{time, session, tool, prompt, why, change, failed}`, which is handy for scripts and editor integrations.

## Limits

- Edits made through shell commands are inferred from the command line and marked `(inferred)`, with the command as the change. tripline recognizes common commands, redirects, git operations, formatters and package managers. It can't see what scripts or code generators do internally.
- Cursor records edits as the agent intended them, without results, so `FAILED` never shows for Cursor. Cursor times come from the transcript file.
- Claude Code may prune old transcripts, and edits from pruned sessions can't be found.
- When the agent's reasoning is hidden, `why` shows its last visible message, which can be empty.
