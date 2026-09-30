# FAQ

**Does tripline send my code or transcripts anywhere?**
No. It reads local files and makes no network calls. The source is small enough to check yourself.

**Does it slow down the agent?**
Barely. tripline re-reads the session transcript on each tool call. On a 3 MB transcript that takes about 90 ms.

**Can it warn instead of blocking?**
Yes. `--warn loop,reread` lets those calls run and tells the agent why they look wrong. `--ask risky` sends destructive commands to your permission prompt instead of blocking them. See [[Rules]]. Blocking stays the default because it's the only response every agent honors.

**tripline blocked something it shouldn't have. What do I do?**
Raise that rule's limit or turn it off with a flag (see [[Rules]]), and please [open an issue](https://github.com/pareshsahoo902/tripline/issues/new/choose) with the blocked call.

**What if tripline itself breaks?**
It fails open. Any internal error allows the call.

**I'm on a subscription, not the API. Is the cost number meaningful?**
It's API-equivalent usage, which is useful for comparing sessions and catching runaway ones. It isn't what you're billed.

**Which agents are supported?**
Claude Code and Codex CLI, fully. Cursor in `scan` and `blame`; a Cursor hook is planned. opencode is next: it stores sessions in SQLite, which takes more work to read. Each agent's parser lives in `internal/transcript`, if you want to help.

**Is the `risky` rule enough to keep me safe?**
No. It catches common destructive commands, but it's a pattern list, not a sandbox. Keep permission prompts on for commands that matter.
