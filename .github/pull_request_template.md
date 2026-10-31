## What and why

<!-- What changed, and the problem it solves. Link the issue if there is one. -->

## How it was tested

<!-- Commands you ran, new tests, fuzz targets. Use generated values only. -->

## Checklist

- [ ] `make check` passes (gofmt, vet, golangci-lint, tests, race detector, examples, govulncheck)
- [ ] Behavior changes are in `docs/spec/v0.1.md`, covered by tests (with a fuzz seed when input is parsed)
      and listed in `CHANGELOG.md`
- [ ] New external facts (formats, algorithms, dates) cite a primary source with an access date
- [ ] No real personal data in code, tests, examples, this description or commit messages; fixtures are
      generated
- [ ] No new runtime dependencies (or an ADR explains why)
- [ ] Every commit is signed off (`git commit -s`)
