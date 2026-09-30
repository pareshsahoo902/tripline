# Hook setup

`tripline hook` runs as a PreToolUse hook. The agent runs it before every tool call, and tripline allows the call, blocks it, asks you, or warns the agent.

## The easy way: `tripline init`

```sh
tripline init                        # Claude Code, every project
tripline init --project              # Claude Code, this project (.claude/settings.json)
tripline init --agent codex          # Codex CLI, every project (~/.codex/hooks.json)
tripline init --agent codex --project
tripline init --budget 5 --ask risky # any rule flag is copied into the hook command
tripline init --remove               # uninstall (add --agent / --project to match)
```

`init` works like this:

- It adds a single `PreToolUse` entry. Other settings and other hooks stay exactly as they were.
- It saves the previous file as `settings.json.bak` (or `hooks.json.bak`).
- It writes atomically, so a crash never leaves a half-written file.
- If tripline is already installed, it updates that entry instead of adding a second one.
- It refuses to touch a file that isn't valid JSON.

Restart the agent afterwards.

**Codex** asks you to review and trust new hooks before running them. Hooks need Codex CLI 0.124 or newer.

## By hand

### Claude Code

Edit `~/.claude/settings.json`. On Windows that's `%USERPROFILE%\.claude\settings.json`. For a single project, use `.claude/settings.json` inside it.

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

### Codex CLI

Edit `~/.codex/hooks.json` (or `$CODEX_HOME/hooks.json`). The format is the same, but add `--agent codex`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [{ "type": "command", "command": "tripline hook --agent codex" }]
      }
    ]
  }
}
```

If the file already has a `hooks` section, add the `PreToolUse` entry to it instead of replacing it.

## Tune the rules

Add flags to the command, or pass them to `init`:

```json
{ "type": "command", "command": "tripline hook --budget 5 --max-repeat 4 --ask risky" }
```

See [[Rules]] for every flag.

## Check that it's active

Restart the agent. Then ask it to run something tripline blocks, for example: *"run `git push --force` in this repo"*. You should see:

```
tripline blocked this call (risky): command matches destructive pattern "git push --force". Ask the user to run it themselves if it is intended.
```

The agent reads this message and responds to it. It won't run the command.

## What the hook receives and returns

The agent sends JSON on stdin that includes `transcript_path`, `tool_name` and `tool_input`. tripline reads the transcript and evaluates the rules. Then it does one of these:

- **Allow:** exits `0` with no output. The agent's normal permission flow applies.
- **Block:** exits `2` with the reasons on stderr. The agent sees them.
- **Ask** (Claude Code): exits `0` and prints `{"hookSpecificOutput":{"permissionDecision":"ask", ...}}`. You decide in the permission prompt.
- **Warn** (Claude Code): exits `0` and prints `additionalContext`, so Claude sees the warning and the call still runs.

tripline never answers `allow`, because that would skip your permission prompts.

If anything unexpected happens (bad input, missing transcript, a parse error), tripline exits `0`. It **fails open**, so a bug in tripline never stalls your agent.
