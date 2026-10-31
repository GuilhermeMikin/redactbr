# Contributing

Thanks for helping improve RedactBR. Bug reports, detector proposals, documentation fixes and focused
improvements are welcome. Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md). The rules
in [CLAUDE.md](CLAUDE.md) apply to people and AI assistants alike; this page covers the practical side.

## Before you start

- **Open an issue first** for anything larger than a small fix, so we can agree on the approach. New
  detectors start with the "Detector proposal" form.
- **Keep the scope.** RedactBR is a small library with strong guarantees. Platforms, dashboards, hosted
  services, databases and full LLM proxies are out of scope; integrations are planned in
  [docs/roadmap.md](docs/roadmap.md).
- **Security problems** go through [SECURITY.md](SECURITY.md), never public issues.
- **No real personal data anywhere:** not in tests, examples, issues, pull request descriptions, commit
  messages or screenshots. Use generated values.

## Setup

You need Go 1.26 or newer and `make`. The Makefile never installs tools; install the linters CI uses with:

```bash
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

golangci-lint also publishes binary releases; see [golangci-lint.run](https://golangci-lint.run/). Both tools must be on your `PATH` (usually `$(go env GOPATH)/bin`).

## Checks

```bash
make check                 # gofmt, vet, golangci-lint, tests, race detector, examples, govulncheck
make test                  # go test ./... (fuzz seeds included)
make race                  # go test -race ./...
make fuzz FUZZTIME=30s     # every fuzz target for 30 s each
make bench                 # go test -run='^$' -bench=. -benchmem ./...
```

GitHub Actions runs the same checks, plus a short fuzzing pass, on every push to `main` and every pull
request. When fuzzing finds a failure, Go writes the input to `testdata/fuzz/<FuzzName>/`; commit that file
with the fix so it becomes a regression test.

## Developer Certificate of Origin (DCO)

This project uses the [Developer Certificate of Origin 1.1](https://developercertificate.org/) instead of a
CLA. Every commit must be signed off:

```bash
git commit -s -m "fix: describe the change"
```

This adds `Signed-off-by: Your Name <you@example.com>`, matching your git identity, and certifies that you
have the right to submit the change under the project's license. To add a missing sign-off to the last
commit, use `git commit --amend -s --no-edit`; for several commits, `git rebase --signoff main`.

## Fixtures

- **Generated, never collected.** Build CPFs and CNPJs from simple, explainable bases (sequential digits,
  repeated pairs, letter patterns) with the check-digit functions; never copy a number from a real
  document, a database, a log or the internet.
- A generated number with valid check digits can still coincide with one that was assigned to a real
  person or company. The rule is "generated, never collected", not "guaranteed to belong to nobody".
- Keep fixture sets small. Do not commit large lists of valid identifiers.
- Official reference vectors published by the Receita Federal may be used in validator tests, with the
  source cited next to them.

## Official sources

Every external fact that enters the repository — a format, a check-digit algorithm, a date, a law, the
behavior of another project — needs a primary source (Receita Federal, Serpro, TSE, CNJ, Ministério da
Saúde, Banco Central, RFCs, go.dev, the project's own repository) with a URL and the access date. If you
cannot find one, say so and mark the fact as `pending official source`; do not fill the gap from memory or
from a blog post.

## Proposing or adding a detector

1. **Issue** with the "Detector proposal" form: the identifier, its official definition (URL), formatted
   and unformatted forms, check-digit algorithm, published test vectors if any, and the false positives you
   expect.
2. **Spec first:** add a row to the entity table in [docs/spec/v0.1.md](docs/spec/v0.1.md) and describe
   shapes, boundaries, normalization and the policy for unformatted values.
3. **Validator** in `internal/<name>` with generated vectors, official vectors when published, and a
   differential test against an independent formulation of the official algorithm when one exists.
4. **Detection:** shapes and policy in the root package; extend the regexp oracle in `oracle_test.go` and
   add fuzz seeds.
5. **Docs:** README table, `CHANGELOG.md`, and the decisions log for any judgment call.

## Pull requests

- One topic per pull request, with what changed and why, and how you tested it. The template walks
  through the checklist.
- Behavior changes update the spec, the tests and `CHANGELOG.md` in the same pull request.
- Style: idiomatic Go, `gofmt`, comments and documentation in English, commit messages in
  [Conventional Commits](https://www.conventionalcommits.org/) (`feat: …`, `fix: …`, `docs: …`).
- No new runtime dependencies without an ADR ([ADR-0002](docs/adr/0002-standard-library-only-core.md)).

## License

By contributing, you agree that your contributions are licensed under the project's
[Apache License 2.0](LICENSE), as certified by your DCO sign-off.
