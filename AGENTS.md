# AGENTS.md

Guidance for AI coding assistants (Codex, Cursor, GitHub Copilot, Claude Code and others) working in this
repository. Humans are welcome to read it too.

**Read [CLAUDE.md](CLAUDE.md) before changing anything.** It is the single source of truth for this
repository's rules, commands and definition of done; this file only points to it so that the two never
diverge.

The rules that matter most, in short:

- Go library, standard library only in the core; small API, no speculative abstractions.
- Fixtures are generated, never collected; no PII in errors, logs, tests, examples, issues or commits.
- Verify external facts (algorithms, laws, dates, other projects) in primary sources and record them.
- Fail closed on internal errors; detection uncertainty is handled by an explicit policy.
- Restore only tokens the session issued; never let pre-existing token-like input gain authority.
- Conventional Commits, `git commit -s` (DCO) on every commit, explicit `git add <path>` only.

If anything here seems to disagree with CLAUDE.md, CLAUDE.md wins; please fix this file.
