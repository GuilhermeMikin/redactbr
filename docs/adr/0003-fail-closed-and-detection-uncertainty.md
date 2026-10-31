# ADR-0003: Fail-closed semantics, kept separate from detection uncertainty

- **Status:** accepted
- **Date:** 2026-10-30

## Context

"Fail closed" is often stated as "when in doubt, redact". That conflates two different situations:

- **A. Internal failures.** The caller asked for redaction and the library could not do it: no
  randomness for a token, a session at capacity, a broken configuration. Returning the input in that case
  would send personal data exactly where the caller tried to prevent it from going.
- **B. Detection uncertainty.** The library works, but cannot know whether `12345678909` is a CPF or an
  order number. Redacting every such value corrupts prompts and answers (amounts, ids, timestamps) and
  makes LLM output wrong in ways that are hard to notice.

## Decision

1. **Internal failures fail closed.** Any operation that can fail returns an error together with an
   **empty result**, never the original text:
   - `New` returns an error for invalid configuration, including an empty entity list.
   - `Session.Redact` returns `("", err)` and leaves the session unchanged on `ErrSessionFull`, random
     source errors and exhausted id collisions.
   - Operations that cannot fail (typed `Redactor.Redact` on a string) have no error result; a future API
     that can fail (for example redaction of an `io.Reader`) must return an error and must not emit
     unredacted data.
   - The package never recovers from a panic to return input unchanged.
2. **Detection uncertainty is an explicit policy** chosen by the caller (`Balanced` by default, or
   `Strict`), documented in [ADR-0005](0005-detection-confidence-policy.md) and the spec. It is not treated
   as an error and is not resolved by redacting everything.

## Consequences

- Callers must handle errors from `Session.Redact`; the examples show "fail closed: send nothing".
- Error messages never contain values (see the spec, §15), so propagating them is safe.
- The default policy has false negatives for bare unformatted numbers; that is documented in the README
  and the spec rather than hidden behind an aggressive default.

## Alternatives considered

- **Return the input on internal error:** rejected; it is the failure mode this library exists to prevent.
- **Panic on internal error:** rejected for expected conditions such as a full session; callers need to
  decide (start a new session, reject the request).
- **"Redact everything when uncertain" as the default:** available as `Strict`; rejected as the default
  because of its effect on LLM correctness.
