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

### D-006 Validators stay internal in v0.1

- `internal/cpf` and `internal/cnpj` are not exported, to keep the public surface small while the API is
  pre-alpha. Exporting them later (for example as `redactbr/brdoc`) is backwards compatible; open question
  for the review, since an alphanumeric CNPJ validator has standalone value.

### D-007 CPF source

- No normative publication of the CPF check-digit algorithm was located. The implementation matches the
  routine `TestaCPF` on the Receita Federal's public CPF status page (spec §3.1), which is an official
  implementation. Recorded as "pending official source" for the normative text.

### D-008 CNPJ sources and an error in the official FAQ

- Algorithm, vectors and format from the Receita Federal DV manual, its reference routines
  (`codigos-cnpj.zip`) and the FAQ (spec §3.2). The manual is byte-for-byte identical to the Serpro PDF.
- FAQ question 23, example 1 (`AA345678/0003-29`) does not satisfy the algorithm (it yields `86`). We
  follow the algorithm and keep the example as a negative vector (spec §3.3).
- The IN RFB 2.229/2024 text could not be rendered (JavaScript-only portal); dates come from the Receita
  Federal project page (published 2024-10-15, in force 2024-10-25, assignments from July 2026).

### D-009 Repeated digits

- Only the all-zero CPF and CNPJ are rejected, matching the official routines. `111.111.111-11` and similar
  are valid candidates and are redacted. Many third-party validators reject repeated digits by convention;
  no official rule was found, and redacting a placeholder is cheap. Reversible.

### D-010 Lowercase alphanumeric CNPJ in detection

- The official format and validator use uppercase letters. Detection folds case before validation in every
  shape (spec §8.1), accepting more false positives under Strict for lowercase codes. Reversible; flagged
  for review.

### D-011 Recognized shapes

- Formatted: `DDD.DDD.DDD-DD`; `XX.XXX.XXX/XXXX-DD`; `XXXXXXXX/XXXX-DD` (used in the official FAQ).
  Unformatted: digits only; `DDDDDDDDD-DD` (a lone check-digit hyphen is common to many identifiers).
  Partial or alternative punctuation is not a candidate. Spec §5.

### D-012 Boundaries

- Letters and digits of any script block a candidate; `_`, `-`, `/` and `,` do not. A `.` next to an ASCII
  digit blocks it (decimals and dotted numbers). The comma stays a boundary so CSV columns remain
  detectable. Spec §6.

### D-013 Context keyword window

- Under Balanced, unformatted values need `cpf`/`cnpj` (ASCII case-insensitive substring) entirely within
  the 32 bytes before the value. Before-only and fixed size for v0.1; making it configurable or adding an
  after-window is an open question. ADR-0005.

### D-014 Token design

- `<ENTITY_id>` with a 10-character lowercase Crockford base32 id (50 bits from `crypto/rand`) per value;
  no counter and no session tag; reservation of look-alikes seen in input; byte-for-byte matching.
  Alternatives and reasoning in ADR-0004. 10 characters rather than 8: under generous assumptions (10^6
  guesses, 10^3 tokens) guessing stays below 10^-6, at a small cost in length.

### D-015 Session capacity

- Default 10,000 entries (distinct values plus reservations) per session, configurable with
  `WithMaxSessionValues`. Bounds memory per conversation; exceeding it fails closed (`ErrSessionFull`).
  Token-like input consumes capacity; accepted because sessions are per conversation (spec §12.5).

### D-016 Session keys and restored form

- CPF/CNPJ keys drop separators and fold case; email keys lowercase only the domain (local parts are
  case-sensitive). Restoration returns the form first seen in the session.

### D-017 Streaming API shape

- `Session.RestoreWriter(io.Writer) *RestoreWriter` with `Write`/`Close`, following `base64.NewEncoder`
  and `gzip.Writer`: it fits push-style SDK streams (write each text delta) and `io.Copy` of plain text.
  It must be fed decoded text, not a raw SSE or JSON body (framing between token halves; JSON escaping of
  `<` and `>`, which Go's `encoding/json` does by default — checked on 2026-10-30). No `io.Reader` variant
  until there is demand.

### D-018 Email scope

- Pragmatic subset of RFC 5322 addr-spec with RFC 5321 length limits (spec §10); no quoted local parts or
  IP literals. Default enabled.

### D-019 slog deferred

- The `slog.Handler` was the first item on the cut list. It is deferred to keep this session's effort on
  the core guarantees; it goes into v0.3 integrations ("observability, slog first").

### D-020 Name check

- No conflict found for `redactbr` on GitHub, pkg.go.dev, proxy.golang.org, PyPI, npm or a web search
  (2026-10-30; details in `docs/prior-art.md`). Trademark registries were not searched.

### D-021 Formatting without contents

- `Session` and `RestoreWriter` implement `String`/`GoString` with counts only; the writer's reusable
  output buffer holds restored values, and `%+v` would otherwise print it. Inspired by obscura's vault.

### D-022 Scanner pre-filter

- Identifier shapes are only tried on runs of ASCII letters and digits whose length and next separator can
  start a shape (2/3 + `.`, 8 + `/`, 9 + `-`, or exactly 11 or 14). About 5x faster on text without
  personal data on the development machine; equivalence checked against the regexp oracle and fuzzing.

### D-023 Fixtures and examples

- Tests, examples and the README use generated values (`123.456.789-09`, `11.223.344/5566-13`,
  `AB.12C.D34/EF56-02`, `ana.silva@example.com`). Official vectors from the Receita Federal appear only in
  validator tests and in two detection tests (FAQ examples), cited.
- A fuzzing regression input (`testdata/fuzz/FuzzRedact/…`, the string `0@0.AA@0.AA`) is committed: it
  made overlapping email matches panic in `Redact` before the fix.

### D-024 Independent review before the first push

An independent review (a separate agent, read-only, with reproductions) ran before the first push. Changes:

- **Zero `Redactor`**: it detected nothing, which contradicted the fail-closed rule. Fields now record
  disabled entities and a zero capacity means the default, so a zero `Redactor` behaves like `New()`.
- **Email**: the domain scan was all-or-nothing, so `ana@example.com-thanks`, `ana@example.com.2` and
  addresses followed by long punctuation were missed, and the `FuzzRedact` Strict invariant could fail in
  CI. Now the longest valid domain prefix wins and limits apply after trimming; `xn--` in any case;
  apostrophes and combining marks in local parts. Other RFC 5322 `atext` characters stay excluded (they
  pull URL text into matches).
- **Memory**: stored values and reservations were substrings that kept whole inputs alive; now copies.
- **Boundaries**: `ª`/`º` are boundaries (`nº123.456.789-09`); combining marks continue words; a `.` right
  after a detected value is a separator, which also fixed a second Strict-invariant failure found while
  fuzzing the fixes (`12345678909.A0000000500000`). Values glued to an ASCII word (`CPF123.456.789-09`)
  stay undetected; documented.
- **Policy**: the CNPJ shape without dots counts as formatted only without lowercase letters (URL path
  segments such as `20261030/main-12` were redacted under Balanced). Keywords may contain dots (`C.P.F.`).
- **Tests**: `FuzzSessionRoundTrip` now uses a low-entropy random source, so removing the reservation makes
  it fail (checked by mutation); new regression seeds; CI prints new fuzz failures; benchmark comments
  corrected and a numbers-heavy benchmark added; `RestoreWriter` formats safely by value and clears its
  reusable output buffer.
- **Docs**: the spec, README, doc.go and CLAUDE.md were corrected where they promised more than the code
  (oracle coverage, "always" for formatted values, SSE bodies, one-pass wording).

---

## 2026-11-06 — Session 2: v0.1 review and hardening

### D-025 A written official source for the CPF check digits

- Found the written rule: `REGRA_VALIDA_CPF` in the e-Financeira manual, Anexo II, versions 2.1.1 and 2.1.2
  (spec §3.1). It supersedes the "no publication located" part of D-007. The worked example (`280012389-38`)
  entered the validator tests as an official vector, and the rule was ported for a differential test.
- Classification, to keep official implementation apart from normative rule: the manual is official
  technical documentation under an Ato Declaratório Executivo Cofis and governs the validation of that
  filing; `TestaCPF` is an official implementation; IN RFB 2.172/2024 is the CPF regulation and **was not
  read**, so a normative definition of the check digits stays `pending official source`.
- Why it was not read: `normas.receita.fazenda.gov.br` and `normasinternet2.receita.fazenda.gov.br` render
  with JavaScript, and the API behind them returned HTTP 403 to anonymous requests. The Diário Oficial search
  did not return the act. The access control was not worked around.
- Details for whoever repeats this: `sped.rfb.gov.br` answers on HTTP only. SHA-256 of the PDFs read:
  `faadc79d1556bdf822c25e9fef4e93522a5626f120d5b030a13d823241e6d4ec` (2.1.2),
  `6b1b1d2494b68bf297fdfd141eaf1aa22a1430117c79ebecec9539e1532354b2` (2.1.1). The site's list of acts and
  the manuals' covers disagree on the dates of the acts (nº 10: 19 and 25 May 2026; nº 9: 16 and 22 April
  2026), so this project cites them by number. The site's "version in production" page still lists 2.1.1;
  the rule's text is identical in both versions.

### D-026 Context after the value and across lists

- `12345678909 é meu CPF` is redacted under Balanced now. The rule after a value is stricter than the rule
  before it (spec §7): no digit, line break, backslash or sentence end in between; a word of its own; not the
  label of what follows.
- Why not the same substring window on both sides. On 36 hand-written sentences whose number has valid check
  digits (16 where the keyword is about the number, 20 where it is not), a symmetric 32-byte window accepted
  15 of the 16 and **all 20** of the others, among them `id=… cpf=…`, `{"id":"…","cpf":"…"}`, `pedido … cpf
  …` and the next line of a log. The adopted rule accepts 15 of the 16 and 1 of the 20. What it gets wrong:
  it keeps `12345678909 é meu CPF: segue` (the colon reads as a label) and it redacts `pedido 12345678909 do
  cliente sem CPF`, which no rule on words can tell apart. These sentences show the shapes that matter; they
  are not a measurement on real traffic, and a wrong acceptance needs the number to have valid check digits
  (one in a hundred).
- A list shares its keyword: a value within 32 bytes after a value of the same entity with context has
  context too, and a keyword after a value also reaches back over the values before it in the same run.
  Before this, `CPFs: a, b, c, d` left `d` in the clear, past the window, and a column under a `cpf` header
  leaked from its fourth row on. A false positive here also needs valid check digits, for each value, so
  lists add little risk. A formatted value without a keyword does not start a list: it says nothing about
  the number after it.
- The window stays at 32 bytes and fixed. The list rule removed the case that argued for a larger one.
- Cost, measured against the previous commit with alternating runs on the development machine: about 12% on
  mixed text under Balanced (about 100 ns per detected value, since each now looks for context) and none
  beyond noise on text without personal data or under Strict.
- Reversible, but it changes what is redacted, so it should settle before `v0.1.0`.

### D-027 Escape sequences written out as two characters

- `"lista:\n123.456.789-09"` was not redacted, even though the value is formatted: `n123` is a run of letters
  and digits, so the value had no left boundary. Tool results often reach models as JSON and the README
  tells callers to send them through the session, so this was a real leak under the default policy.
- The letter of `\n`, `\r` or `\t` no longer glues a value to it, and an email local part starts after it.
  Other escapes are not covered: `\u00a0` before a value and `\/` inside a CNPJ, which JSON allows (RFC 8259
  §7, accessed 2026-11-06), stay undetected; JSON-aware redaction is planned for v0.2.
- A false positive needs a path such as `C:\new…` whose remainder is a whole candidate with valid check digits.

### D-028 Review of the reversible tokens

Reviewed the grammar, collisions, look-alikes, isolation, forgery, mutation, concurrency and streaming against
the code, with new tests and mutation checks.

- **Held up:** byte-for-byte matching (`TestNoSingleByteEditOfATokenRestores` tries every single-byte edit),
  reservation including look-alikes and new values in one call, isolation by issued set even when two sessions
  draw the same id, re-draws on collision, bounded streaming, restoration without re-scanning, and concurrent
  issuing with streams (`TestConcurrentRedactAndStreams`, with ids from a small space).
- **Fixed:** `%+v` of a dereferenced `Session` printed its values, against spec §12.7, so the state now sits
  behind a pointer; `RestoreWriter` ignored short writes that came without an error.
- **Stated plainly:** the docs said what the mechanism does, and readers could take that as more than it is.
  Spec §12.8 now separates accidents from adversaries: nothing protects against someone who repeats a valid
  token, against a model swapping two valid tokens, or against a restored value steered into a link or tool
  call. Tests pin the limits down. The practical advice follows from it: restore only what the reader may see,
  and keep data they must not see out of that session.
- **Left as is:** `Session.Redact` builds its output while holding the write lock. A session belongs to one
  conversation, so contention is low and the change has no measured benefit yet.
- **Token grammar stays provisional** until the fidelity harness runs against real models (D-029).

### D-029 Token fidelity harness

- `internal/tokenfidelity` measures token round trips with LLMs. It is `package main` under `internal/` because
  it is a development tool, not API or a CLI (the CLI is planned for v0.3), and because `go test ./...` then
  covers it. Details and the decision rule for the grammar are in `docs/token-fidelity.md`.
- **Nothing was sent to any provider in this session.** No credentials were used and no paid call was made; all
  results so far come from simulated models and local test servers, which test the harness and not the models.
- Provider calls use `net/http` rather than official SDKs, because ADR-0002 keeps the module free of
  dependencies. A separate module could use the SDKs, at the cost of a second `go.mod` for two small requests.
- Sampling parameters are left out, since API version `2026-10-21` of the Claude API rejects them and a pipeline
  gets the default anyway. No fallback model is configured, so that a run measures the model it names.
- A run is capped (`-max-requests`), plain HTTP is refused except on localhost, redirects are not followed (a
  custom key header would follow them), and the key appears in no report.
