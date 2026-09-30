# Contributing to tripline

Thanks for helping. Bug reports, rule ideas and support for new agents are all welcome.

## Development

You need Go 1.23 or newer.

```sh
git clone https://github.com/pareshsahoo902/tripline
cd tripline
go test ./...
go build .
./tripline scan          # try it on your most recent Claude Code session
```

The code has three parts:

- `internal/transcript` parses agent transcripts into events. Start here to add a new agent.
- `internal/rules` decides whether to block a tool call.
- `internal/blame` links file edits to the prompts that caused them.

`main.go` wires them into the CLI.

## Ground rules

- **The hook fails open.** Code on the `tripline hook` path must never block a tool call because of an internal error. When in doubt, allow.
- **No network.** tripline reads local files only.
- **No new dependencies** without discussing it in an issue first. The standard library has been enough so far.
- **Every rule change needs a test.** For `risky` patterns, add both a command that must be blocked and a similar command that must be allowed.
- Run `gofmt`, `go vet ./...` and `go test ./...` before opening a PR. CI runs the tests on Windows, macOS and Linux.

## Reporting false positives

If tripline blocked something it shouldn't have, open an issue with the blocked command or tool call and the rule named in the message. Redact anything private.

## Releases

Maintainers tag `vX.Y.Z` on `main`. The release workflow builds the binaries and updates the Homebrew tap and Scoop bucket.
