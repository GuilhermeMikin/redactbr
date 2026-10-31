# ADR-0001: Apache-2.0, DCO, and an open-source core that stays open source

- **Status:** accepted
- **Date:** 2026-10-30

## Context

RedactBR is meant to be adopted inside companies (fintech, health, telecom, support software) that review
licenses before adding a dependency, and it may later have commercial components around it. Contributors
need a simple way to certify their contributions, and adopters need to trust that the core will not be
taken away.

## Decision

1. The project is licensed under the **Apache License 2.0** (`LICENSE`, unmodified text from
   <https://www.apache.org/licenses/LICENSE-2.0.txt>, accessed 2026-10-30), with a `NOTICE` file.
   - It is permissive and widely accepted in enterprise environments.
   - Section 3 grants an explicit patent license from each contributor, with termination for anyone who
     sues over patents in the work; section 5 makes contributions submitted to the project fall under the
     same license unless stated otherwise.
2. Contributions use the **Developer Certificate of Origin 1.1** (<https://developercertificate.org/>,
   accessed 2026-10-30): every commit carries a `Signed-off-by:` trailer (`git commit -s`) matching the
   author's identity. There is **no CLA**: contributors keep their copyright, and nobody receives extra
   rights to relicense their work.
3. **The open-source core will not be relicensed into a closed-source license.** Everything related to
   detection, redaction, tokenization, the CLI, integrations, a self-hosted sidecar, middleware and basic
   gateway support stays open source. **Detection quality must never be paywalled.** If commercial
   components exist one day, they are built around the core, not by moving the core or its detectors
   behind a license change.

## Consequences

- Adopters get a permissive license with a patent grant and a public commitment about the core's future.
- Without a CLA, relicensing the existing code would need every contributor's consent; that is intended.
- The DCO must be enforced on pull requests (a check can be added when external contributions start).

## Alternatives considered

- **MIT:** simpler, but without an explicit patent grant.
- **AGPL or BSL:** would discourage exactly the in-process, enterprise use the project targets, and BSL
  is not open source.
- **CLA:** gives the maintainer relicensing power that contradicts decision 3 and adds friction.
