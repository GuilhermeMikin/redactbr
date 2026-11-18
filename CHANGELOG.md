# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Until `v0.1.0` the API may change without notice.

## [Unreleased]

## [0.0.1] - 2026-11-17

First public preview. The API and the token format may still change before `v0.1.0`, which waits on
measuring token fidelity with real models ([roadmap](docs/roadmap.md)).

### Added

- Detection of CPF (formatted and unformatted), numeric CNPJ and alphanumeric CNPJ (IN RFB 2.229/2024),
  with check digits validated against the Receita Federal's published algorithms and reference vectors.
- Email detection (a pragmatic rule based on RFC 5322, with the RFC 5321 length limits).
- Detection policies: `Balanced` (default; unformatted values need context: the entity's keyword within the
  32 bytes before them, the keyword about them within the 32 bytes after them, or a list that shares a
  keyword) and `Strict`.
- `Redactor` with `New`, `WithPolicy`, `WithEntities` and `WithMaxSessionValues`; `Find`, which reports
  entities and offsets without values; typed replacement with `Redact` (`<CPF>`, `<CNPJ>`, `<EMAIL>`).
- Reversible sessions: `Session.Redact` issues `<ENTITY_id>` tokens with 50-bit random ids, reserves
  token-like input it sees so that it never becomes restorable, and fails closed (`ErrSessionFull`);
  `Session.Restore` restores only tokens the session issued.
- Streaming restoration with `Session.RestoreWriter`, which restores tokens split across writes while
  holding back at most 17 bytes.
- A `log/slog` handler, `Redactor.NewSlogHandler`, that redacts messages, keys and values. The name of a field
  counts as context for its value, and numbers are checked too.
- Runnable examples (`examples/basic`, `examples/llm-stream`, `examples/slog`), godoc examples, benchmarks, fuzz
  targets and a regexp-based oracle test.
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
- `fmt.Sprintf("%+v", *session)`, on a dereferenced `Session`, printed its values, and so did `%s`; a `Session`
  now holds them two pointers away. The same goes for the buffers of a `RestoreWriter`.
- Addresses differing only by a Unicode case fold in the domain (`ana@exampie.com`, `ana@exampİe.com`) shared a
  token, so the second restored as the first.
- A whole address stayed in the clear when its local part reached back into an identifier
  (`11.223.344/5566-13.ana@example.com`), when text glued before it made the run longer than 64 bytes, or when its
  top-level label had a digit or a combining mark.
- Formatted values after a list number without a space (`1.123.456.789-09`) were not detected: the rule for
  decimal numbers now applies only to unformatted values.
- `12345678909` was kept in escaped JSON (`{\"doc\":\"12345678909\",\"tipo\":\"CPF\"}`), which
  a backslash cut off from its keyword.
- `RestoreWriter` treated a short write without an error from the underlying writer as a success, which lost
  text silently; it now returns `io.ErrShortWrite`.

[Unreleased]: https://github.com/GuilhermeMikin/redactbr/compare/v0.0.1...HEAD
[0.0.1]: https://github.com/GuilhermeMikin/redactbr/releases/tag/v0.0.1
