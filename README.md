# RedactBR

[![CI](https://github.com/GuilhermeMikin/redactbr/actions/workflows/ci.yml/badge.svg)](https://github.com/GuilhermeMikin/redactbr/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Brazilian PII redaction for Go applications and LLM pipelines.**

Local, reversible, streaming-safe and dependency-free: RedactBR finds CPF, CNPJ (numeric and the new
alphanumeric format) and email addresses in text, inside your process, and replaces them with a typed
label or with a session token that it can restore later — including in an LLM answer that arrives in
chunks. The core uses the Go standard library only.

```bash
go get github.com/GuilhermeMikin/redactbr
```

> **Status: pre-alpha.** The API may change before `v0.1.0`; there are no releases yet. Requires Go 1.26
> or newer (CI tests Go 1.26 and 1.27).

## Example

```go
package main

import (
	"fmt"
	"log"

	"github.com/GuilhermeMikin/redactbr"
)

func main() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}

	// Typed replacement, irreversible.
	fmt.Println(r.Redact("Customer CPF: 123.456.789-09, supplier CNPJ AB.12C.D34/EF56-02, ana.silva@example.com"))
	// Customer CPF: <CPF>, supplier CNPJ <CNPJ>, <EMAIL>

	// Reversible tokens for an LLM round trip.
	session := r.NewSession() // one per conversation
	prompt, err := session.Redact("My CPF is 123.456.789-09")
	if err != nil {
		log.Fatal(err) // fail closed: send nothing
	}
	fmt.Println(prompt) // My CPF is <CPF_k7q2xm9pb3> (random id)

	answer := "Confirmed: " + prompt[len("My CPF is "):] // what a model might answer
	fmt.Println(session.Restore(answer))
	// Confirmed: 123.456.789-09
}
```

Runnable programs: [`examples/basic`](examples/basic) (policies, typed replacement, a reversible round
trip) and [`examples/llm-stream`](examples/llm-stream) (a simulated streamed LLM answer with tokens split
across chunks). `go run ./examples/llm-stream` needs no API key and no network.

## What works today

| Entity | Recognized forms | Validation | Redacted under the default policy |
|---|---|---|---|
| CPF | `123.456.789-09`, `123456789-09`, `12345678909` | Official check digits | Formatted: always. Unformatted: only after the keyword `cpf` |
| CNPJ (numeric) | `11.223.344/5566-13`, `11223344/5566-13`, `11223344556613` | Official check digits | Formatted: always. Unformatted: only after the keyword `cnpj` |
| CNPJ (alphanumeric) | `AB.12C.D34/EF56-02`, `AB12CD34/EF56-02`, `AB12CD34EF5602`, either case | Official check digits (IN RFB 2.229/2024) | Same as numeric |
| Email | `ana.silva@example.com` (pragmatic RFC 5322 subset) | Structure | Always |

All values in this README are generated test values.

- **Typed replacement:** `Redactor.Redact` → `<CPF>`, `<CNPJ>`, `<EMAIL>`.
- **Detection only:** `Redactor.Find` returns entities and byte offsets, never values, so matches are safe
  to log.
- **Reversible sessions:** `Session.Redact` → `<CPF_k7q2xm9pb3>`; `Session.Restore` puts the values back.
- **Streaming restoration:** `Session.RestoreWriter(w)` restores tokens split across writes, holding back
  at most 17 bytes.
- **Policies:** `Balanced` (default) and `Strict`, below.

### Formatted vs. unformatted values

A valid check digit alone is weak evidence: one in a hundred arbitrary 11-digit numbers has valid CPF
check digits, and LLM and agent traffic is full of order numbers, ids and timestamps. So:

- **Formatted** values (`123.456.789-09`, `11.223.344/5566-13`) are redacted whenever their check digits
  are valid.
- **Unformatted** values (`12345678909`) are redacted under the default `Balanced` policy **only when the
  entity's keyword appears in the 32 bytes before them** (`CPF:`, `cpf=`, `"customerCpf":`, `CPF/CNPJ`, in
  any case). A bare unformatted value without a nearby keyword is **not** redacted by default.
- `Strict` redacts every value with valid check digits, formatted or not, and accepts the false positives
  that come with it:

```go
strict, err := redactbr.New(redactbr.WithPolicy(redactbr.Strict))
```

Values with invalid check digits are never redacted, and nothing is detected only because it has the right
length. The exact rules, including boundaries (`x12345678909` and `0.12345678909` are not candidates), are
in the [specification](docs/spec/v0.1.md).

## Why

- **Brazil is a first-class target, not an add-on.** The detectors follow the Receita Federal's published
  algorithms and reference vectors, including the alphanumeric CNPJ that the Receita Federal assigns to
  new registrations from July 2026, and the policy for unformatted Brazilian identifiers is explicit and
  tested.
- **Redaction should be able to happen in-process.** An external PII API adds latency, an operational
  dependency and another system that receives the data you are trying to protect. Those can be the right
  trade-offs; RedactBR gives Go services the option not to make them.
- **LLM traffic needs reversibility.** Models need to refer to "the customer's CPF" and answers need the
  real value back for the user. Tokens are short enough for models to copy, and only tokens the session
  issued are ever restored.

Several projects already cover parts of this, in Go and other languages; see [prior art](docs/prior-art.md).

## LLM pipelines

```text
user text ──Session.Redact──► prompt with tokens ──► LLM provider
                                                          │
user ◄──RestoreWriter (chunk by chunk)── streamed answer ◄┘
```

- Use **one session per conversation** and keep it for the conversation's lifetime: the same value keeps
  the same token across turns, which keeps prompts stable.
- Send **all** model-bound text through `Session.Redact`, including history and tool results.
- Handle errors by sending nothing: `Session.Redact` returns an empty string on error, never the input.
- A model may mangle a token (change its case, drop a character). Mangled tokens are left as they are —
  the user sees the broken token, never a wrong value. Consider telling the model, in the system prompt,
  to copy placeholders such as `<CPF_k7q2xm9pb3>` exactly.

## Security

RedactBR is a data-minimization control for well-intentioned applications. It helps against accidental
disclosure of personal data to LLM providers, logs and other third parties, and it lets answers reuse
tokenized values. It does **not** protect against a user who wants to leak data (spacing digits, writing
numbers in words, invisible characters), data in images or PDFs, names and addresses (no NER), prompt
injection in general, or a compromised process. The full threat model is in the
[specification](docs/spec/v0.1.md#16-threat-model).

Guarantees you can rely on (each covered by tests, fuzzing or both):

- **Fail closed.** Operations that can fail return an error and an empty result, never the unredacted
  input. A configuration that would detect nothing is rejected.
- **Restoration authority.** A session restores only tokens it issued. Tokens from other sessions, forged
  or modified tokens, and token-like text that reached the session as input before it issued that token
  are left verbatim. Ids are 50 random bits from `crypto/rand`.
- **Bounded work.** Detection is a single linear pass; streaming restoration holds at most 17 bytes;
  sessions have a capacity (`WithMaxSessionValues`, default 10,000).
- **No values in diagnostics.** Errors, `Match`, `Session` and `RestoreWriter` print entities, offsets and
  counts, never values.
- **Concurrency.** `Redactor` and `Session` are safe for concurrent use (tested with `-race`); a
  `RestoreWriter` serves one stream.

A session is a trust scope: anything it tokenized can be restored into any text given to it. Never share
a session across users. Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

RedactBR can support LGPD data-minimization measures; using it does not make a system compliant.

## Limitations

- Unformatted values without a nearby keyword are not redacted under the default policy (see above).
- Not recognized: digits separated by spaces or partial punctuation (`123 456 789 09`, `123.456789-09`),
  full-width digits, values with invisible characters inside, numbers in words.
- Email: no quoted local parts, IP literals or single-label domains. Typed or tokenized, an email loses
  its domain too.
- Only CPF, CNPJ and email. Other Brazilian identifiers (CNH, PIS, CNS, título de eleitor, phone numbers,
  CEP) and payment cards are planned, not implemented.
- Token survival in real models has not been measured yet.
- The CPF check-digit rule matches the Receita Federal's own validation routine; a normative publication of
  the algorithm was not located (see the spec).

## Technical decisions

- **Standard library only** in the core, so adding RedactBR adds no transitive dependencies
  ([ADR-0002](docs/adr/0002-standard-library-only-core.md)).
- **Validation is separate from policy.** Validators only check check digits; the policy decides whether
  a valid candidate is redacted ([ADR-0005](docs/adr/0005-detection-confidence-policy.md)).
- **Fail closed on internal errors, explicit policy for uncertainty**, instead of "redact everything when in
  doubt" ([ADR-0003](docs/adr/0003-fail-closed-and-detection-uncertainty.md)).
- **Random per-value token ids with reservation of look-alikes**, rather than counters (`<CPF_1>`) that
  collide with user text and across sessions ([ADR-0004](docs/adr/0004-reversible-token-session-safety.md)).
- **A hand-written linear scanner** instead of regular expressions, checked against a regexp-based oracle
  and fuzz targets.
- **`io.Writer` for streaming,** like `encoding/base64` and `compress/gzip`, so it works with SDK streams and
  `io.Copy` alike.
- **Apache-2.0 with DCO, no CLA; the open-source core stays open source**
  ([ADR-0001](docs/adr/0001-apache-2.0-and-dco.md)).

More: [specification](docs/spec/v0.1.md), [decisions log](docs/decisions-log.md).

## Tests and benchmarks

```bash
make check                     # gofmt, vet, golangci-lint, tests, race, examples, govulncheck
go test ./...                  # unit, oracle and example tests; fuzz seeds run too
go test -race ./...
make fuzz FUZZTIME=30s         # every fuzz target, 30 s each
go test -run='^$' -bench=. -benchmem ./...
```

Benchmarks cover single values, 1 KB and 10 KB mixed text, text without personal data, tokenization and
(streaming) restoration. Numbers depend on the machine; run them on yours.

## Roadmap

- **v0.1** — this release candidate: CPF, CNPJ (numeric and alphanumeric), email, typed replacement,
  sessions, streaming restoration.
- **v0.2** — more Brazilian identifiers, payment cards, ambiguity resolution, JSON and tool-call arguments.
- **v0.3** — CLI, `net/http` and gRPC middleware, OpenAI-compatible, Anthropic and MCP integrations, a
  sidecar, `slog`.

Details in [docs/roadmap.md](docs/roadmap.md).

## Contributing

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md). Every commit needs a DCO sign-off
(`git commit -s`), detectors need an official source, and fixtures must be generated, never collected.
Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Professional support

Professional support and consulting may be available through [mikin.ai/contact](https://mikin.ai/contact).

## License

[Apache License 2.0](LICENSE). Created by Guilherme Balduino Lopes ([GitHub](https://github.com/GuilhermeMikin),
[mikin.ai](https://mikin.ai)).
