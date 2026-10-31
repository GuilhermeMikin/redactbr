# Decisions log

Smaller decisions, verification notes and session context, oldest first. Decisions with long-term
architectural weight live in [`docs/adr/`](adr/). Each entry says whether it is easy to reverse.

Dates are when the decision was recorded. "Accessed" dates are when a source was opened.

---

## 2026-10-30 — Session 1: project bootstrap

### D-001 Git identity and DCO

- Commits are authored and signed off (`git commit -s`) with the identity already configured in git on the
  maintainer's machine (`user.name` `GuilhermeMikin` and the maintainer's personal email).
- Checked before the first commit: the same identity authors the maintainer's existing public repository
  history (`GuilhermeMikin/ai-portfolio-template`), and GitHub resolves that email to the account
  `GuilhermeMikin` (commit API `author.login`). No identity was invented or changed.
- Reversible: future commits could use another identity; existing sign-offs stay as they are.

### D-002 Module path casing

- `github.com/GuilhermeMikin/redactbr`, matching the canonical login returned by `gh api user`
  (`"login": "GuilhermeMikin"`). The same casing is used in `go.mod`, imports, README, examples and CI.

### D-003 Supported Go versions

- Source: <https://go.dev/doc/devel/release> (accessed 2026-10-30): "Each major Go release is supported
  until there are two newer major releases." Current releases: go1.27.0 (2026-08-19, latest patch go1.27.3,
  2026-10-19) and go1.26.0 (2026-02-10, latest patch go1.26.10, 2026-10-19). Go 1.25 is no longer supported.
- `go.mod` declares `go 1.26.0` (the oldest supported release); CI tests Go 1.26.x and 1.27.x. Declaring an
  older version would claim compatibility that CI does not test.
- The maintainer's machine has go1.25.6 (unsupported). Development in session 1 used
  `GOTOOLCHAIN=go1.27.3` (a toolchain downloaded and verified by the go command into the module cache);
  nothing was changed in the system installation.
- `crypto/rand.Read` "never returns an error, and always fills b entirely" (<https://pkg.go.dev/crypto/rand>,
  accessed 2026-10-30); the minimum Go version guarantees this behavior.
- Reversible.

### D-004 Repository stays private until manual review

- The repository remains private until a manual review confirms the API, README, security properties,
  test suite, benchmark methodology and launch positioning.
- A future public launch requires at least: CPF, numeric CNPJ, alphanumeric CNPJ, actual redaction, tests,
  benchmark, runnable example, coherent README and CI. Tagging `v0.1.0` is a separate, later decision.

### D-005 Development tools

- golangci-lint v2.15.0 and govulncheck v1.9.0 were installed with `go install` into `~/go/bin` (user
  level). They are not module dependencies. CI pins the same versions.
- GitHub Actions are pinned to full commit SHAs (resolved from the release tags with the GitHub API on
  2026-10-30): actions/checkout v7.0.1 `3d3c42e5aac5ba805825da76410c181273ba90b1`, actions/setup-go v7.0.0
  `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`, golangci/golangci-lint-action v9.3.0
  `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a`. Dependabot keeps them current.
