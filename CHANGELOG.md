# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project will use
[Semantic Versioning](https://semver.org/) once it has releases. Until `v0.1.0` the API may change without
notice.

## [Unreleased]

### Added

- Detection of CPF (formatted and unformatted), numeric CNPJ and alphanumeric CNPJ (IN RFB 2.229/2024),
  with check digits validated against the Receita Federal's published algorithms and reference vectors.
- Email detection (a pragmatic subset of RFC 5322 with the RFC 5321 length limits).
- Detection policies: `Balanced` (default; unformatted values need context: the entity's keyword within the
  32 bytes before them, the keyword about them within the 32 bytes after them, or a list that shares a
  keyword) and `Strict`.
- `Redactor` with `New`, `WithPolicy`, `WithEntities` and `WithMaxSessionValues`; `Find`, which reports
  entities and offsets without values; typed replacement with `Redact` (`<CPF>`, `<CNPJ>`, `<EMAIL>`).
- Reversible sessions: `Session.Redact` issues `<ENTITY_id>` tokens with 50-bit random ids, reserves
  token-like input so it can never become restorable, and fails closed (`ErrSessionFull`, randomness
  errors); `Session.Restore` restores only tokens the session issued.
- Streaming restoration with `Session.RestoreWriter`, which restores tokens split across writes while
  holding back at most 17 bytes.
- Runnable examples (`examples/basic`, `examples/llm-stream`), godoc examples, benchmarks, fuzz targets and
  a regexp-based oracle test.
- Specification, ADRs, roadmap, prior-art notes, security policy, contribution guide (DCO) and CI.
- A harness that measures whether LLMs copy tokens faithfully (`internal/tokenfidelity`,
  `docs/token-fidelity.md`). It runs offline by default; no real model has been measured yet.

### Security

Found during development and fixed before any release:

- Overlapping email addresses such as `0@0.AA@0.AA` made `Redact` panic (found by fuzzing).
- A zero `Redactor` (not created with `New`) detected nothing; it now behaves like `New()`.
- Addresses followed by glued text (`ana@example.com-thanks`, `ana@example.com.2`) or long punctuation
  were not redacted, and `sant'anna@…` or decomposed accents were redacted only in part.
- Sessions kept whole inputs in memory through substrings of stored values; they now store copies.
- `CPF nº123.456.789-09` was not detected; lowercase URL path segments shaped like a CNPJ without dots
  were redacted under the default policy.
- Values right after a line break or tab written out as two characters, as in JSON strings and logs
  (`"lista:\n123.456.789-09"`), were not detected, even formatted: the letter of the escape glued to them.
- In a list after one keyword (`CPFs: a, b, c, d`), values past the 32-byte window were not redacted.
- `12345678909 é meu CPF` was not redacted: only a keyword before a value counted.
- `fmt.Sprintf("%+v", *session)`, on a dereferenced `Session`, printed its values; a `Session` now holds its
  state behind a pointer.
- `RestoreWriter` treated a short write without an error from the underlying writer as a success, which lost
  text silently; it now returns `io.ErrShortWrite`.
