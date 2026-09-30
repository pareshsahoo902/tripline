# Hook setup

`tripline hook` is a Claude Code [PreToolUse hook](https://code.claude.com/docs/en/hooks). Claude Code runs it before every tool call, and tripline either allows the call or blocks it.

## Guard every project

Edit `~/.claude/settings.json`. On Windows that's `%USERPROFILE%\.claude\settings.json`.

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

If the file already has a `hooks` section, add the `PreToolUse` entry to it instead of replacing it.

## Guard one project

Put the same JSON in `.claude/settings.json` inside the project. Commit it if your team should share the guard.

## Tune the rules

Add flags to the command:

```json
{ "type": "command", "command": "tripline hook --budget 5 --max-repeat 4" }
```

See [[Rules]] for every flag.

## Check that it's active

Restart Claude Code. Then ask it to run something tripline blocks, for example: *"run `git push --force` in this repo"*. You should see:

```
tripline blocked this call (risky): command matches destructive pattern "git push --force". Ask the user to run it themselves if it is intended.
```

Claude reads this message and responds to it. It won't run the command.

## What the hook receives

Claude Code sends JSON on stdin that includes `transcript_path`, `tool_name` and `tool_input`. tripline reads the transcript, evaluates the rules, then:

- exits `0` to allow the call, or
- exits `2` with a reason on stderr to block it. Claude sees the reason.

If anything unexpected happens (bad input, missing transcript, a parse error), tripline exits `0`. It **fails open**, so a bug in tripline never stalls your agent.
