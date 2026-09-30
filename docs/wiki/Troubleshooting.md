# Troubleshooting

## The hook never blocks anything

1. Run `tripline version` in a **new** terminal. If you get "command not found", the binary isn't on your `PATH`. Claude Code runs hooks with your normal shell environment.
2. Check that `settings.json` is valid JSON and that the `PreToolUse` entry sits under `"hooks"`.
3. Restart Claude Code after changing settings.
4. Test the hook by hand:

   ```sh
   echo '{"transcript_path":"<path to any session .jsonl>","tool_name":"Bash","tool_input":{"command":"rm -rf ~"}}' | tripline hook
   echo $?    # should print 2
   ```

   On Windows, use a path like `C:/Users/me/.claude/projects/.../x.jsonl`. Git Bash paths such as `/c/Users/...` don't work with a native binary, and tripline allows the call when it can't open the file.

## `scan` says "no transcripts found"

Claude Code keeps sessions in `~/.claude/projects`. If you set `CLAUDE_CONFIG_DIR`, tripline looks in `$CLAUDE_CONFIG_DIR/projects` instead. Use `--dir` to point it elsewhere.

## `blame` finds nothing

- Run it from the project folder, or pass an absolute path.
- The file may have been changed through shell commands rather than edit tools. See the limits on [[Blame]].

## macOS says the binary can't be opened

The Homebrew cask removes the quarantine flag for you. If you downloaded the binary by hand, run:

```sh
xattr -d com.apple.quarantine /usr/local/bin/tripline
```
