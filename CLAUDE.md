# CLAUDE.md

Guidance for AI coding assistants (Claude Code, Codex, Cursor, GitHub Copilot and others) and for humans
working in this repository. [AGENTS.md](AGENTS.md) points here: this file is the single source of truth for
the project's rules. Do not copy these rules into other files; link to this one.

## The project in one paragraph

RedactBR is a Go library that redacts Brazilian PII (CPF, numeric and alphanumeric CNPJ) and email
addresses in-process, for Go applications and LLM pipelines. It offers typed replacement (`<CPF>`),
reversible per-session tokens (`<CPF_k7q2xm9pb3>`) and streaming restoration of those tokens in model
output. The core uses the Go standard library only. The project is **pre-alpha**: the API may change before
`v0.1.0`. The behavior contract is [docs/spec/v0.1.md](docs/spec/v0.1.md).

## Commands

```bash
make test        # go test ./...
make race        # go test -race ./...
make vet         # go vet ./...
make lint        # golangci-lint run (install it first, see CONTRIBUTING.md)
make vuln        # govulncheck ./... (install it first, see CONTRIBUTING.md)
make fmt-check   # fails if gofmt would change a file
make bench       # go test -run='^$' -bench=. -benchmem ./...
make fuzz        # runs every fuzz target for FUZZTIME (default 20s each)
make examples    # runs the programs in examples/
make check       # everything above except bench and fuzz
```

The Makefile never installs anything. Go 1.26 or newer is required (`go.mod`); CI tests the supported Go
releases.

## Rules

### Scope and design

- **Standard library only** in the module's packages ([ADR-0002](docs/adr/0002-standard-library-only-core.md)).
  Development tools (golangci-lint, govulncheck) are installed and run separately and never appear in
  `go.mod`. A runtime dependency needs an ADR first.
- **Small surface area, strong guarantees.** No speculative abstractions, no interfaces with a single
  implementation, no factories, no `utils` package, no global mutable state. Unexported until there is a
  reason to export. Packages under `internal/` are implementation details.
- **Spec → tests → implementation.** A behavior change starts in `docs/spec/v0.1.md`, then tests, then
  code. README and examples must only show APIs that compile today.
- **Out of scope for now:** dashboards, control planes, SaaS, billing, tenants, databases, Redis, Kafka,
  Kubernetes, full LLM proxies, MCP servers. See [docs/roadmap.md](docs/roadmap.md).

### External facts

- Every external factual claim that enters the repository (check-digit algorithms, document formats, laws,
  dates, Go support policy, behavior of other projects, security or performance claims) must be verified in
  a primary source **in the session that adds it**, with the URL and access date recorded (spec, ADR,
  `docs/prior-art.md` or `docs/decisions-log.md`).
- Prefer primary sources: Receita Federal, Serpro, TSE, CNJ, Ministério da Saúde, Banco Central, go.dev,
  RFCs, the official repository of the project being described. Blogs help find sources; they do not replace
  them.
- If a fact cannot be verified, do not write it as fact: mark it `pending official source` and add it to
  the open questions.
- Describe other projects only from pages actually opened. Never diminish them; if they already solve
  something we plan, say so.

### Privacy and fixtures

- **Fixtures are generated, never collected.** Never use, collect or paste real documents, real people's
  data or production logs. A generated identifier with valid check digits can still coincide with one that
  was assigned to someone, so the rule is "generated, never collected", not "guaranteed to belong to
  nobody". Keep fixture sets small and explainable; never commit large lists of valid identifiers.
- Official reference vectors (published by Receita Federal in its documentation and reference code) may be
  used in validator tests, with the source cited next to them.
- **No PII in errors, logs, panic messages, benchmark names or debug output.** Errors name the entity, a
  category and, when safe, a byte offset — never the value. Library code never prints values; tests use
  only generated fixtures and official vectors, so their failure messages may show those.
- No real PII in tests, examples, issues, pull request descriptions or commit messages.

### Security semantics

- **Fail closed on internal failure** ([ADR-0003](docs/adr/0003-fail-closed-and-detection-uncertainty.md)):
  when a caller asks for redaction and the operation fails, return an error and an empty result — never
  the original input. Never `recover()` a panic to hand back unredacted text.
- **Detection uncertainty is a policy decision**, not "redact everything": `Balanced` (default) and
  `Strict`. A checksum match alone is not enough under `Balanced` for unformatted values. Nothing is ever
  detected only because it has the right length.
- **Token safety** ([ADR-0004](docs/adr/0004-reversible-token-session-safety.md)): only tokens issued by a
  session are restored by that session; token-like text that reached the session as input before the
  session issued it never gains restoration authority; unknown, modified or foreign tokens are left
  verbatim; sessions never restore each other's tokens; randomness comes from `crypto/rand`.
- **Bounded work:** scanning is linear in the input; streaming restoration buffers at most one token's
  length; sessions have a capacity limit and store copies (`strings.Clone`), never substrings that would
  keep whole inputs alive.
- **Concurrency:** `Redactor` and `Session` are safe for concurrent use; `RestoreWriter` is not (one per
  stream). Every change touching shared state must pass `go test -race ./...`.

### Git

- [Conventional Commits](https://www.conventionalcommits.org/) (`feat: …`, `fix: …`, `docs: …`, `test: …`,
  `ci: …`, `chore: …`).
- **DCO on every commit:** always `git commit -s`. The `Signed-off-by` trailer must match the identity
  configured in git (`git config user.name` / `user.email`). Never invent or change an identity.
- **Stage explicit paths only.** Never `git add .`, `git add -A` or `git commit -a`.
- Before every commit run `git status --short`, `git diff`, `git diff --cached` and
  `git diff --cached --name-only`, and confirm that `CLAUDE.local.md` (private, gitignored) is not staged.
- No tool attribution in commits or pull requests: no `Co-Authored-By` lines for AI tools, no "Generated
  with/by …". The only special trailer is `Signed-off-by`.
- Do not push, tag, create releases or change repository visibility unless the owner asks for it in the
  current session.
- Leave `main` clean: no scratch files, no temporary worktrees or branches, no secrets.

### Language

- Code, comments, documentation, commit messages, GitHub metadata, examples and templates are in English.

## Where things live

| Path | Contents |
|---|---|
| `doc.go`, `redactor.go` | Package documentation, `Redactor`, options, `Find`, `Redact` |
| `scan.go`, `email.go` | Candidate scanning, boundaries, context and the detection policy |
| `session.go`, `token.go` | Reversible sessions, token grammar and generation |
| `restore.go` | `RestoreWriter` (streaming restoration) |
| `internal/cpf`, `internal/cnpj` | Check-digit validators (no detection policy) |
| `examples/` | Runnable programs; CI runs them |
| `docs/spec/v0.1.md` | Behavior contract: entities, formats, policy, tokens, limits |
| `docs/adr/` | Architecture decision records |
| `docs/decisions-log.md` | Smaller decisions and session notes, newest last |
| `docs/roadmap.md`, `docs/prior-art.md` | Plan and ecosystem notes |

## Definition of done

`make check` passes (gofmt, vet, golangci-lint, tests, race, examples, govulncheck); new behavior is in the
spec and covered by tests (including a fuzz seed when it parses input); README, spec and `CHANGELOG.md`
match the code; no real PII, secrets or private files were added; every commit is signed off.
