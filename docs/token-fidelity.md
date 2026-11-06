# Token fidelity

> **Status: no real model has been measured.** The harness below is ready and tested offline. A run against a
> provider needs the owner's authorization, because it can cost money and uses provider credentials. Until
> then the token grammar (spec §12.1) stays provisional.

Reversible tokens work only if a model copies them byte for byte: a mangled token stays mangled in front of the
reader (spec §12.4). The format is also expensive to change after `v0.1.0`, since callers' prompts and parsing will
depend on it. One project, pii-mask, reports models reformatting bracketed placeholders
([prior art](prior-art.md)), so this needs numbers, not assumptions.

## What the harness does

[`internal/tokenfidelity`](../internal/tokenfidelity) runs 12 fixed scenarios (repeat a line, confirm data,
summarize, extract JSON, build a table, translate, rewrite, list many values, answer from history, pick one of
two values, and two stress cases: HTML and all caps), each with and without a system instruction asking the
model to copy placeholders exactly. For each run it:

1. tokenizes the generated text with a new `Session`, and stops if a value is left in what would be sent;
2. sends it to the model;
3. classifies what happened to each token in the answer;
4. restores the answer with `Session.Restore` and with `RestoreWriter` in small chunks, and compares them.

It uses only the public API and the standard library (ADR-0002), so provider calls are plain `net/http` rather
than an SDK.

## Running it

Offline, with no key and no network. These numbers come from scripted fake models, test the harness, and say
**nothing** about real models:

```bash
make fidelity                                  # or: go run ./internal/tokenfidelity
```

Against a provider, the harness first prints a plan and sends nothing. It reads no key and opens no connection
without `-live`:

```bash
go run ./internal/tokenfidelity -provider anthropic -model MODEL_ID -repeats 12 -max-requests 288
```

To run the plan, for real and possibly at a cost:

```bash
export ANTHROPIC_API_KEY=…                     # or OPENAI_API_KEY with -provider openai
go run ./internal/tokenfidelity -provider anthropic -model MODEL_ID -repeats 12 -max-requests 288 \
  -live -out fidelity-MODEL_ID.jsonl
```

- `-provider openai` also reaches OpenAI-compatible servers through `-base-url`, including local ones, which
  need no key and cost nothing beyond the machine: `-base-url http://localhost:11434/v1` for Ollama.
- `-max-requests` (default 100) refuses larger runs. Keys go only to `https` URLs or `localhost`, redirects
  are not followed, and keys do not appear in reports or in `-out`.
- With a key not scoped to a workspace, the Claude API needs `ANTHROPIC_WORKSPACE_ID` too.

### What is missing to run it for real

1. The owner's authorization, in the session that runs it.
2. A provider key in the environment, or a local server.
3. The list of models to measure.
4. A budget. The plan prints the number of requests and the ceiling on output tokens (requests × `-max-tokens`).
   Answers should be much shorter than the ceiling, though no real run has shown it yet, and thinking counts
   towards it on models that think.

With `-repeats 12`, a variant gets 324 tokens in the ordinary group, for 288 requests per model. If none breaks,
the 95% upper bound on the failure rate is about 1% (the rule of three, treating tokens as independent; tokens in
one answer are not quite).

## What it reports

| Column | Meaning |
|---|---|
| `complete` | Answers in which every expected value came back after restoration |
| `tokens` | Expected tokens in the answers |
| `restorable` | Tokens present byte for byte, so the reader gets the value |
| `changed` | How tokens were changed, with counts: `mixed` (byte for byte in one place, changed in another), `case`, `escaped` (`&lt;`, `\u003c`), `spacing`, `separator`, `brackets` (removed or replaced), `other`, `id_edited` (an id within two edits of the real one) |
| `missing` | Tokens with nothing recognizable in the answer |
| `invented` | Well-formed tokens the session did not issue |
| `wrong_token` | Tokens the scenario did not ask for that showed up. In `choose_one_pt` this is a model choosing the wrong person; spec §12.8 says nothing detects that in production |
| `raw_value` | Answers with a value before restoration. It should always be 0 |
| `failed`, `refused`, `truncated` | Provider errors, refusals, and answers cut by the token limit |
| `stream_diff` | Answers where `RestoreWriter` and `Restore` disagree. It should always be 0 |

Reports split `ordinary` from `stress` scenarios. Stress scenarios give a model a reason to change a token, so
they show how tokens break, not how often. `-out` records one JSON object per request: the scenario, tokens and
answer, never restored values. Its first line has the scenario hash, model and settings. All values are
generated, so the file can be shared.

## How the result decides the grammar

Proposed, to be settled **before** looking at results:

- **Freeze `<ENTITY_id>`** if, for every model measured, the `instruction` variant of the `ordinary` group has at
  least 99% `restorable` and no `raw_value`. Read the answers with `invented` and `wrong_token` by hand: they say
  something about the models, and no token format changes them.
- **Otherwise, compare alternatives** before `v0.1.0`, with the same scenarios: other delimiters, a shorter id, or
  a tolerant restoration of specific, harmless changes. The `changed` column says which change to look at.
  Tolerance has a cost: anything that matches more than the exact token widens what can be forged (spec §12.8).
- **Tell callers** what the results say about the system instruction. The README suggests it; this measures
  whether it helps.

## Limits of the method

- Hand-written scenarios with generated values, not production traffic. They find failure modes and give a rough
  rate; they do not predict the rate of a specific application.
- Default sampling and default settings, since that is what a pipeline gets. Results age with model versions.
- Tokens in one answer are correlated, and scenarios are not a random sample of anything.
- Non-streaming requests: models are not expected to write differently when streaming, but that is an assumption.
  Tool calls are not covered. Restoration across arbitrary chunk boundaries is tested elsewhere (`restore_test.go`),
  with synthetic splits.
- No fallback model is configured, on purpose: a refused request is reported, not answered by another model.

## Results

| Date | Provider and model | Requests | `ordinary`, no instruction | `ordinary`, instruction | Notes |
|---|---|---|---|---|---|
| — | none yet | — | — | — | — |

## Sources for the request shapes

Accessed 2026-11-06.

- Claude API: [API overview](https://platform.claude.com/docs/en/api/overview) (endpoint, headers, workspace
  header; it calls `x-api-key` a legacy fallback for `Authorization` that is still supported, and the harness uses
  it as the current examples do), [Messages reference](https://platform.claude.com/docs/en/api/messages/create)
  (top-level `system`; thinking counts towards `max_tokens`), [versions](https://platform.claude.com/docs/en/api/versioning) (`2026-10-21`; `temperature`, `top_p`
  and `top_k` removed), [Messages guide](https://platform.claude.com/docs/en/build-with-claude/working-with-messages)
  (request, `x-api-key` example, response), [stop reasons](https://platform.claude.com/docs/en/build-with-claude/handling-stop-reasons)
  (`refusal`, `max_tokens`).
- OpenAI: [Chat Completions reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
  (endpoint, `Authorization`, `max_completion_tokens`, `max_tokens` deprecated, `choices[].message.content` and
  `refusal`, `finish_reason`) and the official [OpenAPI specification](https://github.com/openai/openai-openapi)
  (`usage.prompt_tokens`, `usage.completion_tokens`).
- Ollama: [OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility) (base URL, `max_tokens`, key
  required by clients but ignored).
