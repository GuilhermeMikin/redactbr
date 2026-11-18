# RedactBR

[![CI](https://github.com/GuilhermeMikin/redactbr/actions/workflows/ci.yml/badge.svg)](https://github.com/GuilhermeMikin/redactbr/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/GuilhermeMikin/redactbr.svg)](https://pkg.go.dev/github.com/GuilhermeMikin/redactbr)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Brazilian PII redaction for Go applications and LLM pipelines.**

RedactBR finds CPF, CNPJ (numeric and the new alphanumeric format) and email addresses in text, inside your
process. It replaces them with a typed label, or with a session token that it can restore later, even in an LLM
answer that arrives in chunks. The core uses only the Go standard library.

> **Status: pre-alpha.** The API may change before `v0.1.0`; `v0.0.1` is a preview release. Requires Go 1.26
> or newer (CI tests Go 1.26 and 1.27).

## Contents

- [Quick start](#quick-start)
- [Why RedactBR](#why-redactbr)
- [What it detects](#what-it-detects)
- [API at a glance](#api-at-a-glance)
- [LLM pipelines](#llm-pipelines)
- [Security model](#security-model)
- [Limitations](#limitations)
- [Technical decisions](#technical-decisions)
- [Tests and benchmarks](#tests-and-benchmarks)
- [Roadmap](#roadmap)
- [Contributing and security](#contributing-and-security)
- [License](#license)

## Quick start

```bash
go get github.com/GuilhermeMikin/redactbr
```

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

Runnable programs, none of which needs an API key or a network:

- [`examples/basic`](examples/basic): policies, typed replacement and a reversible round trip.
- [`examples/llm-stream`](examples/llm-stream): a simulated streamed LLM answer with tokens split across chunks.
- [`examples/slog`](examples/slog): redacted logs.

## Why RedactBR

- **Brazil first, not an add-on.** Check digits follow the Receita Federal's published algorithms and reference
  vectors, including the alphanumeric CNPJ assigned to new registrations from July 2026. The policy for
  unformatted Brazilian identifiers is explicit and tested.
- **In-process, standard library only.** An external PII API adds latency, an operational dependency and
  another system that receives the data you are trying to protect. Those can be the right trade-offs; RedactBR
  gives Go services the option not to make them, with no module dependencies.
- **Reversible, streaming-safe tokens.** Models can refer to "the customer's CPF" while the user still gets the
  real value back. Tokens use 50 random bits from `crypto/rand`, only tokens the session issued are restored,
  and `RestoreWriter` restores tokens split across chunks while holding back at most 17 bytes.
- **Safe by default.** It fails closed and never prints values in errors or diagnostics
  ([security model](#security-model)).
- **Tested hard, cheap on clean text.** 99% statement coverage in the main package, 8 fuzz targets, a
  regexp-based oracle, the race detector and govulncheck in CI. About 500 MB/s with zero allocations on 10 KB
  of text without personal data ([benchmarks](#tests-and-benchmarks)).

Several projects already cover parts of this, in Go and other languages; see [prior art](docs/prior-art.md).

## What it detects

| Entity | Recognized forms | Validation | Redacted under the default policy |
|---|---|---|---|
| CPF | `123.456.789-09`, `123456789-09`, `12345678909` | Official check digits | Formatted: yes. Unformatted: only with the keyword `cpf` nearby |
| CNPJ (numeric) | `11.223.344/5566-13`, `11223344/5566-13`, `11223344556613` | Official check digits | Formatted: yes. Unformatted: only with the keyword `cnpj` nearby |
| CNPJ (alphanumeric) | `AB.12C.D34/EF56-02`, `AB12CD34/EF56-02`, `AB12CD34EF5602`, either case | Official check digits (Receita Federal's manual) | Same as numeric; `ab12cd34/ef56-02` (lowercase, no dots) counts as unformatted |
| Email | `ana.silva@example.com` (a pragmatic rule based on RFC 5322) | Structure | Always |

All values in this README are generated test values. The official CNPJ format uses uppercase letters. Detection
accepts either case, because people type lowercase and it identifies the same company ([spec §8.1](docs/spec/v0.1.md#81-lowercase-alphanumeric-cnpj)).

### Formatted vs. unformatted values

A valid check digit alone is weak evidence: one in a hundred arbitrary 11-digit numbers has valid CPF check
digits, and LLM and agent traffic is full of order numbers, ids and timestamps. So:

- **Formatted** values (`123.456.789-09`, `11.223.344/5566-13`) are redacted whenever their check digits are
  valid and they are not glued to letters or digits (`CPF123.456.789-09` is not detected;
  `CPF nº123.456.789-09` is).
- **Unformatted** values (`12345678909`), and the partially formatted `123456789-09`, are redacted under the
  default `Balanced` policy **only with context**. A bare value without it is **not** redacted by default.

| Text | Balanced | Why |
|---|---|---|
| `CPF: 12345678909` | `CPF: <CPF>` | keyword in the 32 bytes before (`cpf=`, `"customerCpf":`, `CPF/CNPJ`, in any case, also `C.P.F.`) |
| `12345678909 é meu CPF` | `<CPF> é meu CPF` | keyword after, in the same sentence and line |
| `CPFs: 12345678909, 98765432100, …` | `CPFs: <CPF>, <CPF>, …` | a list shares its keyword |
| `id=12345678909 cpf=98765432100` | `id=12345678909 cpf=<CPF>` | `cpf=` labels what follows it, not the `id` |
| `order 12345678909 shipped` | unchanged | no context |

`Strict` redacts every value with valid check digits, formatted or not, and accepts the false positives that
come with it:

```go
strict, err := redactbr.New(redactbr.WithPolicy(redactbr.Strict))
```

Values with invalid check digits are never redacted, and nothing is detected only because it has the right
length. The exact rules, including boundaries (`x12345678909` and `0.12345678909` are not candidates), are in
the [specification](docs/spec/v0.1.md).

## API at a glance

| Call | What it does |
|---|---|
| `Redactor.Redact` | Typed, irreversible replacement: `<CPF>`, `<CNPJ>`, `<EMAIL>` |
| `Redactor.Find` | Entities and byte offsets, never values, so matches are safe to log |
| `Session.Redact` / `Session.Restore` | Reversible tokens (`<CPF_k7q2xm9pb3>`) and their restoration |
| `Session.RestoreWriter(w)` | Restores tokens split across writes in a model's answer, holding back at most 17 bytes |
| `Redactor.NewSlogHandler(next)` | Redacts `log/slog` records |
| `WithPolicy`, `WithEntities`, `WithMaxSessionValues` | Options for `New`: policy, entities and session capacity |

- `RestoreWriter` is about **restoring output**: redaction takes whole strings, and redacting a stream of input
  is not implemented.
- In the `slog` handler, the name of a field counts as context, so `slog.String("cpf", "12345678909")` and
  `slog.Int64("cpf", 1234567890)`, which lost a leading zero, are redacted. Errors and structs are logged as
  redacted text.

Full reference on [pkg.go.dev](https://pkg.go.dev/github.com/GuilhermeMikin/redactbr).

## LLM pipelines

```text
user text ──Session.Redact──► prompt with tokens ──► LLM provider
                                                          │
user ◄──RestoreWriter (chunk by chunk)── streamed answer ◄┘
```

- Use **one session per conversation** and keep it for the conversation's lifetime: the same value keeps the
  same token across turns, which keeps prompts stable.
- Send **all** model-bound text through `Session.Redact`, including history and tool results, as whole strings.
  `Session.Redact` does not take a stream, so a value split across two calls is not detected.
- **Restore only what the reader of the answer may see.** A session restores everything it tokenized, so data the
  reader must not see belongs in `Redactor.Redact`, which is irreversible, and not in that session.
- Handle errors by sending nothing: `Session.Redact` returns an empty string on error, never the input.
- Restore **decoded text**: write the model's text deltas to `RestoreWriter`, not the raw SSE or JSON body.
  A token split across events has framing between its halves, and JSON encoders may escape `<` and `>`
  (Go's `encoding/json` does by default), which hides the token.
- A model may mangle a token (change its case, drop a character). Mangled tokens are left as they are, so the
  user sees the broken token. A change would give a wrong value only by producing another token of the session,
  which 50 random bits make negligible. Consider telling the model, in the system prompt, to copy placeholders
  such as `<CPF_k7q2xm9pb3>` exactly.

## Security model

RedactBR is a data-minimization control for well-intentioned applications. It helps against accidental
disclosure of personal data to LLM providers, logs and other third parties, and it lets answers reuse
tokenized values. It does **not** protect against a user who wants to leak data (spacing digits, writing
numbers in words, invisible characters), data in images or PDFs, names and addresses (no NER), prompt
injection in general, or a compromised process. The full threat model is in the
[specification](docs/spec/v0.1.md#16-threat-model).

Guarantees you can rely on (covered by tests, fuzzing or both, except the linear scan, which benchmarks track):

- **Fail closed.** Operations that can fail return an error and an empty result, never the unredacted input. A
  configuration that would detect nothing is rejected, and a zero `Redactor` detects everything.
- **Against accidents and guessing.** A session restores only tokens it issued, matched byte for byte. Tokens from
  other sessions, forged or modified tokens, and token-like text that reached the session as input before it issued
  that token are left verbatim.
- **Bounded work.** Detection is a linear scan plus a sort of the matches; streaming restoration holds at most
  17 bytes; sessions have a capacity (`WithMaxSessionValues`, default 10,000) and store copies of values, never
  the inputs they came from.
- **No values in diagnostics.** Errors, `Match`, `Session` and `RestoreWriter` print entities, offsets and
  counts, never values.
- **Concurrency.** `Redactor`, `Session` and the `slog` handler are safe for concurrent use (tested with `-race`);
  a `RestoreWriter` serves one stream.

What the tokens do **not** protect against ([spec §12.8](docs/spec/v0.1.md#128-accidents-and-adversaries)):

- **Someone repeating a valid token.** A token is a reference, not an authorization. A user who saw one, or a
  model told to repeat it through prompt injection, gets the value restored. A session cannot tell who wrote a
  text, so it is a trust scope: never share one across users.
- **A value steered somewhere else.** Restoration replaces text and does not know where a token is, so a token
  inside a link or a tool call becomes a real value there. Restore as the last step, and treat the result as
  model output that now contains real data.
- **A model mixing tokens up.** Swapping two customers' tokens restores both faithfully, in the wrong places.

RedactBR can support LGPD data-minimization measures; using it does not make a system compliant.

## Limitations

- **Coverage:** only CPF, CNPJ and email. Other Brazilian identifiers (CNH, PIS, CNS, título de eleitor, phone
  numbers, CEP) and payment cards are planned, not implemented.
- **Not recognized:** digits separated by spaces or partial punctuation (`123 456 789 09`, `123.456789-09`),
  values glued to a word (`CPF123.456.789-09`), full-width digits, values with invisible characters inside,
  percent-encoded text (`cpf%3D12345678909`), numbers in words. JSON escapes other than `\n`, `\r` and `\t`
  hide a value even under `Strict` (`11.223.344\/5566-13`, a value right after `\f` or ` `).
- **Email:** no quoted local parts, IP literals or single-label domains. Typed or tokenized, an email loses its
  domain too.
- **Leading zeros:** a CPF that lost its leading zeros, as spreadsheets do, is not recognized in text, only in
  `slog` number fields.
- **Input:** must be whole strings. Redacting a stream of input, and JSON-aware redaction, are planned.
- **Sessions:** live in the memory of one process. They do not survive a restart or move between replicas, and a
  new session does not restore the tokens of an old one.
- **Token fidelity:** how faithfully real models copy tokens has not been measured yet; [a harness is ready](docs/token-fidelity.md).
- **CPF rule source:** the CPF check-digit rule is the one the Receita Federal documents in its e-Financeira
  manual and has written in a routine that its public CPF page loads. Those are official technical sources; a
  normative definition is still pending (see the spec).

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
- **`io.Writer` for streaming,** like `encoding/base64` and `compress/gzip`, so it fits SDK streams that
  deliver text deltas as well as `io.Copy` of plain text.
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

A sample of the benchmarks, measured with Go 1.26.0 on an AMD Ryzen 7 4800H (linux/amd64), median of three runs:

| Benchmark | Input | Time/op | Throughput | Allocs/op |
|---|---|---|---|---|
| `RedactNoPII10KB` | 10 KB without personal data | 19.6 µs | 526 MB/s | 0 |
| `Mixed10KB` | 10 KB, half the lines with a CPF, a CNPJ and an email | 89 µs | 115 MB/s | 15 |
| `RedactCPF` | one formatted CPF in a short sentence | 515 ns | 72 MB/s | 2 |
| `Tokenize` | 1 KB mixed text, new session | 17 µs | 61 MB/s | 44 |
| `RestoreStreaming` | 10 KB answer written in 16-byte chunks | 46.5 µs | 219 MB/s | 6 |

Numbers depend on the machine; run them on yours.

## Roadmap

- **v0.1:** in progress. `v0.0.1` ships CPF, CNPJ (numeric and alphanumeric), email, typed replacement,
  sessions, streaming restoration and a `log/slog` handler. `v0.1.0` waits on measuring token fidelity with
  real models and freezing the token format.
- **v0.2:** more Brazilian identifiers, payment cards, ambiguity resolution, JSON and tool-call arguments.
- **v0.3:** CLI, `net/http` and gRPC middleware, OpenAI-compatible, Anthropic and MCP integrations, a sidecar.

Details in [docs/roadmap.md](docs/roadmap.md).

## Contributing and security

Contributions are welcome: see [CONTRIBUTING.md](CONTRIBUTING.md). Every commit needs a DCO sign-off
(`git commit -s`), detectors need an official source, and fixtures must be generated, never collected.
Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

Report vulnerabilities privately as described in [SECURITY.md](SECURITY.md), not in a public issue.

Professional support and consulting may be available through [mikin.ai/contact](https://mikin.ai/contact).

## License

[Apache License 2.0](LICENSE). Created by Guilherme Balduino Lopes ([GitHub](https://github.com/GuilhermeMikin),
[mikin.ai](https://mikin.ai)).
