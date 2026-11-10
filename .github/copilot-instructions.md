# Instructions for AI coding assistants

Read [`AGENTS.md`](../AGENTS.md) first: the layout, rules and verification commands live there only.

- Run `make test lint contract` before proposing a change. The command-line contract in `test/contract` is frozen.
- Only `internal/term` writes to the terminal; cobra lives under `internal/cli`; calls to the platform and to GitHub
  live in `internal/platform` and `internal/selfupdate`.
- Use conventional commit subjects. This repository is public: keep ticket keys, internal hostnames and internal
  repository names out of everything you write.
