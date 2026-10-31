# ADR-0004: Reversible token session safety

- **Status:** accepted
- **Date:** 2026-10-30

## Context

A session replaces values with tokens before text goes to an LLM and restores them in the answer. Finding
the string `<CPF_1>` in an answer is not, by itself, a reason to restore anything:

- **Pre-existing look-alikes.** A user writes "Please explain `<CPF_1>`". Later the session tokenizes a
  real CPF and happens to choose `<CPF_1>`. The model repeats the user's text, and a naive restore turns
  the user's literal into the real CPF.
- **Cross-session confusion.** Two sessions with counters both issue `<CPF_1>`; an answer produced under
  one session and restored with the other mixes people's data.
- **Forgery and mutation.** A model (or a user, through the model) produces tokens that were never issued,
  or changes one slightly.
- **Concurrency.** Several requests of one conversation can tokenize at the same time.

Tokens also have to be easy for models to copy and cheap to parse in a stream.

## Decision

1. **Token format:** `<ENTITY_id>`, where `id` is 10 characters of lowercase Crockford base32 (`0-9a-z`
   without `i l o u`), **drawn from `crypto/rand` for every new value**: 50 bits, no counter, no session
   tag. Maximum length 18 bytes. Grammar in the spec, §12.1.
2. **Only issued tokens are restored,** matched byte for byte against the session's own issued set.
   Unknown, forged, modified and foreign tokens are left verbatim. A session never restores another
   session's tokens, and ids are independent across sessions.
3. **Reservation:** every well-formed token in the input of `Session.Redact` that the session has not
   issued is reserved before any new token is issued, and the session never issues a reserved string.
   This makes the pre-existing look-alike case impossible for text that passes through the session, not
   just improbable. Issued tokens found in input (history sent again) are left alone.
4. **Stable tokens:** equal values (normalized) share one token for the session's lifetime; restoration
   returns the value as first seen.
5. **Bounded state:** a capacity (default 10,000 values plus reservations) with `ErrSessionFull`; a
   failed call leaves the session unchanged (ADR-0003).
6. **Concurrency:** one mutex per session; `Redact` holds the write lock for the whole transition, restore
   paths hold the read lock for lookups.
7. **A session is a trust scope.** The documentation says plainly that everything a session tokenized can
   be restored into any text given to it, so sessions are per conversation and never shared across users.

## Consequences

- Guessing an issued token succeeds with probability about *n*/2⁵⁰ per guess (*n* tokens of that entity in
  the session); a look-alike that bypasses the session collides with that probability instead of never.
- Token-like input consumes capacity, so whoever controls a session's input can make it fail closed.
  Sessions are per conversation, so the effect stays within that conversation.
- Tokens differ between sessions, so the same CPF cannot be linked across conversations through tokens;
  callers who need stable pseudonyms across sessions need a different, keyed scheme (roadmap).
- Random tokens make outputs non-deterministic; tests use an internal hook to fix the random source.

## Alternatives considered

- **Sequential tokens (`<CPF_1>`):** easiest for models, but collide with user text, across sessions and
  with tokens leaked from other contexts.
- **Session tag plus counter (`<CPF_1_k7q2>`):** readable, but one leaked token reveals the tag needed to
  forge the others, and tokens get longer.
- **HMAC of the value with a session key:** deterministic and verifiable, but longer, and with a shared
  key it links values across sessions.
- **Escaping look-alikes in the input:** changes user text and relies on the model preserving the escape.
- **Authenticated tokens (MAC suffix):** stronger against forgery, but much longer tokens that models copy
  less reliably; forgery is already bounded by the issued-set check, and the real boundary is the session
  scope.
