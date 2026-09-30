# Security policy

tripline runs on every tool call an AI agent makes, so security bugs matter here. Two kinds are especially important:

- A way to make the hook allow a destructive command it is designed to block, such as an `rm -rf ~` variant that slips past the `risky` rule.
- Anything that makes tripline read or write outside the Claude Code transcripts it is pointed at.

## Reporting

Please report vulnerabilities privately through [GitHub security advisories](https://github.com/pareshsahoo902/tripline/security/advisories/new), not in public issues. You should get a reply within 7 days.

## Scope

The `risky` rule is a safety net, not a sandbox. A determined agent can write a destructive command in a form no pattern list will catch. Bypass reports are still welcome and will be fixed, but don't rely on tripline as your only protection. Keep Claude Code's permission prompts on for commands you care about.
