# Rules

All four rules run on every tool call. Set any numeric flag to `0` to turn its rule off.

## `loop`

**Blocks** a tool call identical to one already made N times with no file edit in between. Identical means the same tool with the same input. Key order in the input doesn't matter.

**Why:** the agent runs `npm test`, it fails, and without changing anything it runs `npm test` again. And again.

**Resets** when the agent edits any file through `Edit`, `MultiEdit`, `Write` or `NotebookEdit`. After an edit, rerunning the tests is legitimate.

**Flag:** `--max-repeat N` (default 3, so the 4th identical call is blocked).

## `reread`

**Blocks** a `Read` of a file, with the same offset and limit, that already happened N times while the file hasn't been edited since.

**Why:** re-reading a file the agent already has in context costs tokens and adds nothing.

**Resets** when the agent edits *that* file. Edits to other files don't count.

**Flag:** `--max-rereads N` (default 2).

**Caveat:** tripline only sees edits made through agent tools. If you edit the file yourself, or a shell command changes it, the agent may be blocked from reading the new version. It's told why and can ask you. If this happens often, raise the limit or set it to `0`.

## `budget`

**Blocks** every tool call once the estimated session cost reaches the limit. The agent is told to stop and ask you.

**Flag:** `--budget USD` (default off).

**How cost is estimated:** tripline sums input, output, cache-write and cache-read tokens from the transcript, counting each API message once, and prices them at Anthropic API list prices for the model used. Unknown models are priced as Sonnet. On a Pro or Max subscription this is API-equivalent usage, not money you're charged.

## `risky`

**Blocks** `Bash` and `PowerShell` commands that match a destructive pattern:

| Pattern | Examples |
|---|---|
| Recursive delete of root, home, cwd or everything | `rm -rf /`, `rm -rf ~`, `rm -rf *`, `rm -rf .` |
| Force push | `git push --force`, `git push -f` (`--force-with-lease` is allowed) |
| Discarding work | `git reset --hard`, `git clean -f…` |
| Pipe-to-shell | `curl … \| sh`, `wget … \| bash`, `irm … \| iex` |
| Disk-level | `mkfs`, `dd … of=/dev/…`, `format C:`, `rd /s /q C:\` |
| Permissions | `chmod -R 777 /` |
| SQL | `DROP TABLE`, `DROP DATABASE`, `DROP SCHEMA` |
| PowerShell | `Remove-Item -Recurse` on a drive root or home |

Deleting ordinary folders (`rm -rf node_modules`, `rm -rf ./build`) is allowed.

**Flag:** `--no-risky` turns it off.

**This is a safety net, not a sandbox.** An agent can phrase a destructive command in ways no pattern list catches. Keep Claude Code's permission prompts on for anything that matters. If you find a bypass, report it; see [SECURITY.md](https://github.com/pareshsahoo902/tripline/blob/main/SECURITY.md).

## Testing thresholds without blocking anything

Run [[Scan]] with the flags you're considering. It shows where each rule *would* have fired in a past session:

```sh
tripline scan --max-repeat 2 --budget 3
```
