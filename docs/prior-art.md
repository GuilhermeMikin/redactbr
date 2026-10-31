# Prior art

What already exists, what RedactBR can learn from it, and how it differs. The goal is ecosystem awareness,
not manufactured differentiation: where another project already does something RedactBR plans, this page
says so.

Everything below was opened on **2026-10-30** (repository pages, READMEs, documentation, LICENSE files
or GitHub license metadata, pkg.go.dev). Details change quickly; check the linked pages before relying on
them. No code was copied from any of these projects.

## Summary

| Project | Language | License | Brazilian CPF/CNPJ in free text | Alphanumeric CNPJ | Reversible tokens | Streaming restoration |
|---|---|---|---|---|---|---|
| [Presidio](https://github.com/data-privacy-stack/presidio) | Python | MIT | Not built in (open PR adds both) | In that PR | Encrypt/decrypt operators | Not found |
| [Google Cloud Sensitive Data Protection](https://docs.cloud.google.com/sensitive-data-protection/docs/infotypes-reference) | Managed API | Proprietary service | CPF (`BRAZIL_CPF_NUMBER`); no CNPJ infoType found | No | Yes (re-identification) | Not documented |
| [tarja](https://github.com/macmaia/tarja) | Python | Apache-2.0 | Yes (17 Brazilian identifiers) | Yes | Yes (`Vault`) | Not found |
| [leakspok](https://github.com/Prosus-Cyber-Xchange/leakspok) | Go | Apache-2.0 | Yes | Yes | No | No |
| [llm-dlp](https://github.com/yurivenancio30/llm-dlp) | Go (proxy application) | MIT | Yes | Yes | Yes (keyed pseudonyms) | Yes, including split pseudonyms |
| [obscura](https://github.com/mattevans/obscura) | Go | MIT | No | No | Yes | Yes, including split placeholders |
| [pii-redaction-middleware](https://github.com/muhammadwaqasmbd/pii-redaction-middleware) | Go | MIT | No | No | Yes | Redaction of streams only |
| [anonde](https://github.com/anonde-io/anonde) | Go | Apache-2.0 | No | No | Yes | Planned |
| [wuming](https://github.com/taoq-ai/wuming) | Go | MIT | Yes | No (digits only) | No | No |
| [LiteLLM Presidio guardrail](https://docs.litellm.ai/docs/proxy/guardrails/pii_masking_v2) | Python | MIT outside `enterprise/` | Custom recognizers only | No | Yes (`output_parse_pii`) | Buffers the response |
| [LLM Guard](https://github.com/protectai/llm-guard) (archived) | Python | MIT | No | No | Yes (`Vault`) | Example re-scans accumulated text |
| [cockroachdb/redact](https://github.com/cockroachdb/redact) | Go | Apache-2.0 | n/a (different problem) | n/a | n/a | n/a |

Brazilian document **validators** (no detection in free text): [paemuri/brdoc](https://github.com/paemuri/brdoc)
(Go, Unlicense, accepts the alphanumeric CNPJ), [klassmann/cpfcnpj](https://github.com/klassmann/cpfcnpj)
(Go, Apache-2.0, digits only), [validate-docbr](https://github.com/alvarofpp/validate-docbr) (Python, MIT),
[brutils-python](https://github.com/brazilian-utils/python) (Python, MIT) and
[brazilian-utils](https://github.com/brazilian-utils/javascript) (TypeScript, MIT; alphanumeric CNPJ behind
an option).

**Bottom line.** Free-text detection of Brazilian identifiers including the alphanumeric CNPJ already
exists (tarja in Python; leakspok and llm-dlp in Go), and so does restoring placeholders split across
stream chunks (llm-dlp and obscura in Go). RedactBR's contribution is the combination in one importable,
standard-library-only Go library: Brazil-first detection with an explicit policy for unformatted values,
reversible session tokens with a deterministic guarantee against look-alike input, and bounded streaming
restoration. No project reviewed here was found to combine all of these; that is an observation about
the projects reviewed, not a claim about the whole ecosystem.

## Notes

### Microsoft Presidio

Now developed at [data-privacy-stack/presidio](https://github.com/data-privacy-stack/presidio) (the
`microsoft/presidio` URL redirects there). Python framework: regex, checksum and NER recognizers with
context words that raise scores, and anonymizer operators (`replace` with `<ENTITY_TYPE>` by default,
`redact`, `hash`, `mask`, `encrypt`, `custom`, `keep`), with `decrypt` for reversal and a pseudonymization
sample. No Brazilian recognizers are built in; open pull request
[#2281](https://github.com/data-privacy-stack/presidio/pull/2281) adds CPF and CNPJ, including the
alphanumeric CNPJ.

- **Learn:** documented overlap rules between recognizers; context words as a confidence signal.
- **Difference:** Python service or library with NER; RedactBR is an in-process Go library with a smaller,
  rule-based scope.

### Google Cloud Sensitive Data Protection (DLP)

Managed API. Has `BRAZIL_CPF_NUMBER` (and `BRAZIL_RG_NUMBER`); no CNPJ infoType appears in the infoTypes
reference. Findings carry a likelihood from `VERY_UNLIKELY` to `VERY_LIKELY`, filtered with
`minLikelihood`, and hotword rules adjust likelihood near keywords. Reversible de-identification (AES-SIV,
format-preserving encryption) with surrogate infoType annotations, reversed through `reidentify`.

- **Learn:** hotword proximity is the same idea as RedactBR's keyword window; self-describing surrogates
  can be restored without offsets.
- **Difference:** a hosted API (an extra network hop and trust boundary) with key-based reversal; RedactBR
  runs in-process.

### tarja

Zero-dependency Python library for LLM and RAG use that finds, validates and masks 17 Brazilian
identifiers in Portuguese text (CPF, numeric and alphanumeric CNPJ, CNS, CNJ, CNH, título and others), with
tiered scores using context words and a second pass for "suspect" matches. `Vault.protect()`/`reveal()`
issue HMAC tokens scoped to the call that issued them. A Presidio plugin exists.

- **Learn:** broad Brazilian coverage and context scoring; restoration scoped to the issuing call.
- **Difference:** Python, no streaming restoration found. The closest peer outside Go.

### leakspok (iFood / Prosus)

Go library that detects CPF and CNPJ ("numeric and alphanumeric formats with validation"), email, cards
and IPs in strings or byte slices, with redact and mask strategies and per-rule exceptions; a companion
anonymizer service targets AI prompts.

- **Learn:** per-rule exceptions for known safe values.
- **Difference:** one-way redaction (no reversible tokens documented); third-party dependencies. It
  overlaps directly with RedactBR's detection layer.

### llm-dlp

Local proxy for Claude Code, built in Brazil. Masks prompts, files and tool output, detects CPF/CNPJ
(including `12.ABC.345/01DE-35`), RG, CNH and NF-e keys, uses keyed pseudonyms that are stable across
sessions, and restores responses "including when a pseudonym arrives split between two chunks".

- **Learn:** deterministic masking of re-sent history keeps provider prompt caching valid (RedactBR gets
  the same effect by keeping one session per conversation).
- **Difference:** an application, not an importable library; pseudonyms are linkable across sessions by
  design, RedactBR's tokens are not.

### obscura and pii-redaction-middleware

[obscura](https://github.com/mattevans/obscura) is a pure-Go library with `⟦EMAIL_1⟧`-style placeholders
and `Vault.NewRestoreStreamer()`, which restores placeholders split across delta boundaries; an HTTP
transport restores SSE and fails closed. Detection covers English-speaking locales, not Brazil.
[pii-redaction-middleware](https://github.com/muhammadwaqasmbd/pii-redaction-middleware) is a
standard-library Go middleware with an `[EMAIL_1]` vault, `Restore`, and tests that split input at every
byte offset.

- **Learn:** RedactBR adopted the every-offset split test; obscura's vault avoids leaking through logging,
  which led RedactBR to format sessions and writers without their contents.
- **Difference:** obscura already implements a streaming design close to RedactBR's, without Brazilian
  identifiers; counter placeholders are predictable, which is what RedactBR's reservation and random ids
  address.

### anonde

Presidio-style Go engine with optional NER; `<EMAIL_ADDRESS_1>` tokens; revealing a value requires an
actor and a purpose. No Brazilian locale; streaming de-anonymization is planned.

- **Learn:** audit metadata on restoration.
- **Difference:** service-oriented; no Brazilian identifiers.

### wuming

Zero-dependency Go library with many locales; its `br` locale covers CPF, CNPJ, phone, CEP, PIS and CNH,
with an LGPD preset, confidence scores and redact/mask/hash.

- **Learn:** locale presets.
- **Difference:** one-way; its CNPJ detector accepts digits only.

### LiteLLM Presidio guardrail

LiteLLM proxy guardrail with `pre_call`, `post_call`, `logging_only` and `pre_mcp_call` modes, per-entity
mask or block, score thresholds, and `output_parse_pii` to restore per-request `<ENTITY_n>` tokens. In the
source, OpenAI-style streams are buffered, rebuilt, unmasked and re-emitted.

- **Learn:** guardrails at tool-call (MCP) boundaries.
- **Difference:** a Python proxy; streaming restoration buffers instead of working incrementally.

### LLM Guard (archived)

`Anonymize` keeps originals in a `Vault` and inserts `[REDACTED_PERSON_1]`; `Deanonymize` restores them.
The repository is archived.

- **Learn:** a preamble that tells the model what the placeholders are and to keep them intact.
- **Difference:** Python, English and Chinese entities, archived.

### cockroachdb/redact

Go library that marks unsafe values at formatting time (`‹ ›` markers, `SafeFormatter`) so logs can be
redacted later. It does no detection.

- **Learn:** explicit safe/unsafe annotation is the right tool when the code knows which values are
  sensitive; detection is for text the code does not control.

### Gateways and MCP proxies

[Bifrost](https://github.com/maximhq/bifrost) (Go, Apache-2.0) documents an enterprise redaction guardrail
whose placeholders are reversible only for log viewing and which holds back tool-call argument deltas
until complete. [Portkey gateway](https://github.com/Portkey-AI/gateway) (TypeScript, MIT) calls a hosted
guardrail API and substitutes the result. [Lasso MCP Gateway](https://github.com/lasso-security/mcp-gateway)
(Python, MIT) sanitizes MCP tool traffic with secrets and Presidio plugins.

- **Learn:** tool-call arguments need to be complete before they are scanned; that informs the v0.2
  structured-data work.

### Brazil-focused Python pseudonymizers

[pii-tokens](https://github.com/oLucasAguilar/pii-tokens), [lgpd-guard](https://github.com/arthurpenedo/lgpd-guard)
and [pii-mask](https://github.com/Rafaelcarvalho320/pii-mask) (all MIT) replace CPF, CNPJ and other values
with tokens for LLM use. pii-mask reports moving from `[CPF_1]` to opaque tokens because models reformatted
the brackets, and offers a way to list tokens the model dropped.

- **Learn:** measure how well tokens survive real models, and report missing tokens. RedactBR has not
  measured token survival yet.

## Name check: `redactbr`

On 2026-10-30: no GitHub repository, user or organization named `redactbr`; no module on pkg.go.dev or
proxy.golang.org; no package on PyPI or npm; no product with that name found by web search. No serious
conflict was found. Trademark registries (INPI, USPTO) were not searched.

## Not verified

INPI/USPTO trademarks; streaming behavior of tarja, pii-mask and lgpd-guard beyond their READMEs; how
much of Bifrost's redaction is open source; the entities covered by hosted services (Portkey, Azure AI
Language); whether Google Cloud supports streaming re-identification; the alphanumeric CNPJ claims of
leakspok and pii-tokens beyond their documentation; performance of any of these projects.
