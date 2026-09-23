# Security Policy

## Supported versions

Current `main` and the latest release tag.

## Reporting a vulnerability

Please open a **private** security advisory on GitHub, or email the maintainer listed on the repository profile.
Do not file public issues for undisclosed vulnerabilities.

## Notes

- The agent can run commands and edit files when write mode is enabled.
- Prefer `--dry-run` and review diffs before applying patches.
- Keep Ollama bound to localhost unless you intentionally expose it.
