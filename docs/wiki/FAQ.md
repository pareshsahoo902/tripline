# FAQ

**Does tripline send my code or transcripts anywhere?**
No. It reads local files and makes no network calls. The source is small enough to check yourself.

**Does it slow down the agent?**
Barely. tripline re-reads the session transcript on each tool call. On a 3 MB transcript that takes about 90 ms.

**Why does it block instead of just warning?**
Claude Code only shows the agent a hook's output when the hook blocks the call. A warning would be invisible to the agent, and preventing waste means the agent has to see it. When a call is blocked, the agent gets the reason and usually changes approach.

**tripline blocked something it shouldn't have. What do I do?**
Raise that rule's limit or turn it off with a flag (see [[Rules]]), and please [open an issue](https://github.com/pareshsahoo902/tripline/issues/new/choose) with the blocked call.

**What if tripline itself breaks?**
It fails open. Any internal error allows the call.

**I'm on a subscription, not the API. Is the cost number meaningful?**
It's API-equivalent usage, which is useful for comparing sessions and catching runaway ones. It isn't what you're billed.

**Will you support Codex / Cursor / opencode / Aider?**
That's the plan. The parser is the only part that's specific to Claude Code. See `internal/transcript` if you want to help.

**Is the `risky` rule enough to keep me safe?**
No. It catches common destructive commands, but it's a pattern list, not a sandbox. Keep permission prompts on for commands that matter.
