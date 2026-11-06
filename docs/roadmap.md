# Roadmap

Plans, not promises. Items move between versions as evidence comes in; nothing here is implemented unless
the spec ([docs/spec/v0.1.md](spec/v0.1.md)) says so. Principle for every version: **small surface area,
strong guarantees**. Future plans must not complicate the core.

## v0.1 — Core redaction for Go and LLM responses

Goal: a small, usable, safe library that is convincing on its own.

| Item | Status |
|---|---|
| CPF detection (formatted and unformatted, official check digits) | done |
| Numeric CNPJ detection | done |
| Alphanumeric CNPJ detection (IN RFB 2.229/2024; case-insensitive detection) | done |
| Email detection (pragmatic RFC 5322 subset) | done |
| Detection policy: `Balanced` (default) and `Strict` | done |
| Typed replacement (`<CPF>`) | done |
| Reversible session tokenization (`<CPF_k7q2xm9pb3>`), reservation of look-alikes, capacity | done |
| Streaming restoration (`RestoreWriter`, split tokens, bounded buffer) | done |
| Unit, fuzz, race, differential (oracle) tests; benchmarks; runnable examples; CI | done |
| Harness for token fidelity with real models | done; no real model measured yet, which needs the owner's authorization |
| `slog.Handler` that redacts log records | deferred (cut for this release candidate; see decisions log D-019) |
| `mask` and keyed (HMAC) replacement strategies | not planned for v0.1: would widen the API before there is demand |
| Review of the API, README and security model before the first public release | pending |

Remaining before tagging `v0.1.0` (a separate decision): the manual review above, measuring token fidelity with
real models and freezing the token grammar ([harness](token-fidelity.md)), a normative definition of the CPF
check digits if one can be read (spec §3.1), and a decision on the open questions in the spec.

## v0.2 — Deeper Brazil coverage and structured data (planned)

Every new detector needs an official source for its format and check digits, generated fixtures, a row in
the spec's entity table and a decision on its formatted/unformatted policy.

- **More Brazilian identifiers:** CNH, PIS/NIS, CNS (Cartão Nacional de Saúde), título de eleitor, CNJ
  process numbers, Brazilian phone numbers with valid DDD handling, PIX random keys (UUID) only with
  context, Mercosul license plates, CEP, RG only with context.
- **Ambiguity resolution:** several of these share lengths with CPF or with each other (for example 11
  digits). The validator decides validity; the policy decides which entity, if any, using shape and
  context. Expect a refinement of `Balanced` and `Strict`, not a scoring engine.
- **Payment card numbers:** supported lengths, Luhn, issuer/network rules only where a reliable source
  supports them, and a false-positive strategy (they collide with other long numbers). Deliberately out of
  v0.1: more research, a larger false-positive surface, and little added to the Brazil-first thesis.
- **Structured data:** JSON-aware redaction (values, not keys), tool-call arguments in LLM requests,
  streaming request redaction.
- **Normalization (opt-in):** full-width digits, invisible characters inside values, spaced digits.
- **Quality:** evaluate mutation testing on the validators and the scanner. Candidates seen on
  2026-10-30: [go-gremlins/gremlins](https://github.com/go-gremlins/gremlins) (Apache-2.0) and
  [avito-tech/go-mutesting](https://github.com/avito-tech/go-mutesting) (MIT). Not a CI gate.

## v0.3 — Integrations and gateway use cases (planned)

Integrations live in separate packages or modules so the core stays standard-library-only.

- CLI (`redactbr` over stdin/stdout and files).
- `net/http` middleware and gRPC interceptors.
- OpenAI-compatible and Anthropic integration examples (request redaction, streamed response
  restoration).
- MCP integration (tool inputs and outputs).
- Self-hosted sidecar/proxy.
- Observability integrations (`slog` first).
- A fair, reproducible external benchmark suite against other tools, with published methodology.

## Future / exploratory

Recorded only so they are not forgotten:

- Pluggable NER for names and addresses.
- Latin American identifiers: RUT (Chile), CUIT (Argentina), CURP/RFC (Mexico).
- Persistent token vault for sessions that outlive a process.
- Managed enterprise components around the open-source core (ADR-0001: the core and detection quality stay
  open source).
