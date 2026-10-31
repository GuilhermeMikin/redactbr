# ADR-0002: Standard-library-only core

- **Status:** accepted
- **Date:** 2026-10-30

## Context

RedactBR runs inside applications that handle personal data, often on the request path to an LLM. Every
third-party dependency in that position is code that sees the data, a supply-chain risk, a source of
transitive upgrades and a reason for a security review to say no. The problem itself — scanning text,
modular arithmetic, maps, `io.Writer`, `crypto/rand` — is well covered by the Go standard library.

## Decision

- The module's packages import **only the Go standard library**. `go.mod` has no `require` lines.
- Development tools (golangci-lint, govulncheck) are installed and run outside the module and are never
  added to `go.mod`, including as `tool` directives.
- A future runtime dependency needs a new ADR that explains why the standard library is not enough, what
  data the dependency sees, its license and its maintenance status. Optional integrations that need
  third-party code (for example an SDK adapter) go in separate modules so the core stays dependency-free.

## Consequences

- `go get github.com/GuilhermeMikin/redactbr` adds no transitive dependencies; govulncheck only has to
  consider the standard library and this module.
- Some conveniences are written by hand (the email subset, the token parser). They are small and tested
  directly, including by fuzzing.
- The minimum Go version is the oldest supported Go release (see `docs/decisions-log.md`, D-003), which is
  also the oldest version CI tests.

## Alternatives considered

- **Regex-based detection with `regexp`:** standard library, but the scanner needs custom boundary rules,
  and a hand-written linear scanner was simpler to reason about for worst-case cost. `regexp` is used in
  tests as an independent oracle.
- **Third-party validators for CPF/CNPJ:** small benefit; the algorithms are short and the alphanumeric
  CNPJ support needed to be checked against the official vectors anyway.
