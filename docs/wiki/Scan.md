# Scan

```sh
tripline scan [--max-repeat N] [--max-rereads N] [--budget USD] [--no-risky] [--ask RULES] [--warn RULES] [--dir DIR] [transcript.jsonl]
```

`scan` replays a session through the rules and summarizes it. With no file argument it uses the most recently modified transcript of any supported agent. It detects the format (Claude Code, Codex or Cursor) from the file itself.

```
$ tripline scan --budget 2
C:\Users\me\.claude\projects\d--Project-app\e59f0edf-….jsonl

  17:42:10  budget  Bash go test ./...
    estimated session cost passed $2.00 (now $2.03 at API list prices). Summarize progress and ask the user whether to continue. tripline checks again at $4.00.

prompts      11
tool calls   41 (2 errors)  Write 10, Edit 8, WebSearch 8, Bash 7, Skill 3, ToolSearch 2
tokens       in 52, out 39558, cache write 225484, cache read 2304160
est. cost    $2.84 (API list prices)
```

Each finding shows when the rule would have fired, the rule name, the tool call, and the message the agent would have seen.

Use `scan` to:

- tune rule thresholds before you enable the hook
- audit a session you weren't watching
- compare cost between sessions

Transcripts live here:

| Agent | Location |
|---|---|
| Claude Code | `~/.claude/projects/<project>/<session>.jsonl` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |
| Cursor | `~/.cursor/projects/<project>/agent-transcripts/<id>/<id>.jsonl` |

To scan a specific session, pass its path. For sessions on models without known prices (such as Codex's GPT models), the cost line reads `n/a`.
