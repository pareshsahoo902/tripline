# Rules

All four rules run on every tool call. Set any numeric flag to `0` to turn its rule off.

## `loop`

**Blocks** a tool call identical to one already made N times with no file edit in between. Identical means the same tool with the same input. Key order in the input doesn't matter.

**Why:** the agent runs `npm test`, it fails, and without changing anything it runs `npm test` again. And again.

**Resets** when the agent changes any file: through `Edit`, `MultiEdit`, `Write` or `NotebookEdit`, or through a shell command that writes files (`sed -i`, `>` redirects, `tee`, `mv`, `git checkout`, formatters like `gofmt -w`, package managers, PowerShell `Set-Content`, and so on). After a change, rerunning the tests is legitimate.

**Flag:** `--max-repeat N` (default 3, so the 4th identical call is blocked).

## `reread`

**Blocks** a `Read` of a file, with the same offset and limit, that already happened N times while the file hasn't been edited since.

**Why:** re-reading a file the agent already has in context costs tokens and adds nothing.

**Resets** when the agent changes *that* file, including through a shell command that names it. Changes to other files don't count. A shell command that changes unnamed files (`git checkout main`, `git stash pop`, `patch < fix.diff`) resets every file.

**Flag:** `--max-rereads N` (default 2).

**Caveat:** tripline only sees changes the agent makes. If you edit the file yourself, or a script changes it, the agent may be blocked from reading the new version. It's told why and can ask you. If this happens often, raise the limit or set it to `0`.

## `budget`

**Fires** once each time the estimated session cost passes a multiple of the budget: at $5, $10, $15 and so on for `--budget 5`. The agent is told to summarize its progress and ask you whether to continue. After you say yes, it can carry on until the next multiple.

**Flag:** `--budget USD` (default off).

**How cost is estimated:** tripline sums input, output, cache-write and cache-read tokens from the transcript, counting each API response once, and prices them at Anthropic API list prices for the model used. Unknown Claude models are priced as Sonnet. Models from other providers, such as Codex's GPT models, have no prices, so the budget never fires for them. On a Pro or Max subscription this is API-equivalent usage, not money you're charged.

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

## Actions: block, ask or warn

Every rule blocks by default. You can change that per rule:

| Flag | What happens | Who sees the reason |
|---|---|---|
| (default) | The call is blocked | The agent |
| `--ask RULES` | The agent's permission prompt asks you | You |
| `--warn RULES` | The call runs, and the agent is told why it looks wrong | The agent (and you) |

`RULES` is a comma-separated list, for example `--ask risky,budget --warn reread`. tripline never auto-approves a call.

`ask` and `warn` need Claude Code. Codex runs the tool anyway when a hook answers anything but block, so with `--agent codex`, `ask` becomes block and `warn` does nothing.

`ask` relies on Claude Code's permission prompt. In modes that can't prompt, such as non-interactive `claude -p`, what happens is up to Claude Code. For rules that must hold everywhere, keep the default.

## Testing thresholds without blocking anything

Run [[Scan]] with the flags you're considering. It shows where each rule *would* have fired in a past session:

```sh
tripline scan --max-repeat 2 --budget 3
```
