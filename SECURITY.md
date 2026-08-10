# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in go-hotspot, please report it
responsibly:

1. **Do not** open a public GitHub issue.
2. Email the maintainer directly.
3. Include a clear description of the vulnerability and steps to reproduce.

You will receive a response within 48 hours. If the vulnerability is confirmed,
a fix will be prioritized and a security advisory will be published.

## Scope

go-hotspot is a read-only analysis tool. It reads source files and runs `git log`.
It does not:

- Execute user-supplied code
- Write to files outside the `--output` path
- Make network requests
- Require elevated privileges

The attack surface is limited to:
- Malicious git repositories with crafted commit metadata (parsed as strings)
- Symlink attacks via `--paths` pointing to sensitive files (read-only)
