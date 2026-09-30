
tripline adds guardrails and blame to AI coding agent sessions. It runs locally as a single binary and never touches the network.

It does three things:

- **Guard** (`tripline hook`): blocks a bad tool call before it runs. That covers loops, re-reads of unchanged files, runaway cost, and destructive shell commands.
- **Explain** (`tripline blame <file>`): shows why the agent changed a file, with the prompt and the agent's own explanation for each edit.
- **Audit** (`tripline scan`): summarizes a finished session and shows where the rules would have fired.

Supported today, on Windows, macOS and Linux:

| | Claude Code | Codex CLI | Cursor |
|---|---|---|---|
| `hook` (guard) | yes | yes (0.124+) | not yet |
| `scan`, `blame` | yes | yes | yes |

## Start here

1. [[Installation]]
2. [[Hook setup|Hook-Setup]]
3. [[Rules]]: what gets blocked, and how to tune it

## Reference

- [[Blame]]
- [[Scan]]
- [[FAQ]]
- [[Troubleshooting]]
