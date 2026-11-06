# ADR-0005: Detection confidence policy for unformatted identifiers

- **Status:** accepted
- **Date:** 2026-10-30; amended 2026-11-06 (context after the value and across lists; escape sequences)

## Context

A valid check digit is weak evidence on its own. For CPF and CNPJ, one in a hundred arbitrary candidates
of the right length has valid check digits, and the text RedactBR sees — prompts, tool calls, logs — is
full of numeric and alphanumeric ids, timestamps, amounts and codes. Formatting, on the other hand, is
strong evidence: `123.456.789-09` with valid check digits is very unlikely to be anything but a CPF.

## Decision

1. **Shapes are explicit** (spec §5). Formatted: the canonical masks, plus the CNPJ variant without dots
   (`XXXXXXXX/XXXX-DD`), which keeps the CNPJ-specific separators and appears in the Receita Federal FAQ —
   unless it has lowercase letters, which makes it look like a URL path segment.
   Unformatted: digits only, plus the CPF variant `DDDDDDDDD-DD`, whose lone hyphen is a generic
   check-digit separator. Other punctuation patterns are not candidates.
2. **Boundaries** (spec §6): no letter, digit or combining mark of any script next to a candidate (the
   ordinal indicators `ª`/`º` and the letter of a written-out `\n`, `\r` or `\t` excepted), and no `.` that
   would make it part of a decimal or dotted number, unless that `.` follows a detected value.
3. **Validity:** official check digits; the all-zero number rejected; repeated digits otherwise accepted.
4. **Policy:**
   - `Balanced` (default): formatted and valid → redact. Unformatted and valid → redact only with context,
     which is one of: the entity's keyword (`cpf`, `cnpj`; any case; substring; dots allowed, as in
     `C.P.F.`) within the 32 bytes before the value; the keyword within the 32 bytes after it, under stricter
     conditions (same sentence and line, no digit in between, a word of its own, not the label of what
     follows); or a list, where values of the same entity within 32 bytes of each other share that context.
   - `Strict`: every valid candidate with clean boundaries → redact.
5. **No hidden scoring.** There is no numeric confidence score in v0.1; the rules above are the whole
   policy, and `Match` does not expose a score.

## Consequences

- Balanced misses bare unformatted values (a CPF alone on a line, CSV columns with a distant header, URL
  path segments). The README states this; Strict exists for callers who prefer recall.
- `12345678909 é meu CPF` and `CPFs: a, b, c, d` are redacted whole. Before the amendment the first was kept
  and the fourth value of the second leaked, past the window.
- Strict redacts ids that happen to have valid check digits (the basic example shows an order number
  redacted as a CNPJ).
- Keyword matching as a substring catches `customerCpf` and `nr_cpf` before a value; the 32-byte window
  keeps unrelated numbers later in a sentence from inheriting context. The same matching after a value would
  turn `id=12345678909 cpf=…` into a false positive, hence the stricter rule there.
- Adding identifiers that share lengths (v0.2) will need disambiguation by keyword and shape; this policy
  layer is where it goes.

## Alternatives considered

- **Checksum only:** about 1% false positives on every candidate-length number; rejected as a default.
- **Numeric scores and thresholds (as Presidio and Google Cloud Sensitive Data Protection do):** more
  flexible, but harder to reason about and to test; may be revisited when there are more entities.
- **A symmetric window after the value:** the same substring rule on both sides. It catches as many true
  positives as the adopted rule (15 of 16 in D-026) and also takes `id=…`, `{"id":…,"cpf":…}`, `pedido … cpf …` and the next line
  of a log as context; decisions log D-026 has the comparison.
- **A larger window:** catches longer labels, and more unrelated numbers. Lists were the case that needed it,
  and the list rule covers them without widening the window for everything else.
