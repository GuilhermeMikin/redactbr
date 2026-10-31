# Security policy

RedactBR exists to keep personal data from leaking, so a way around one of its documented guarantees is a
security bug even when nothing crashes.

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately through GitHub:
[Security → Report a vulnerability](https://github.com/GuilhermeMikin/redactbr/security/advisories/new).
If that form is not available, contact the maintainer through [mikin.ai/contact](https://mikin.ai/contact)
and ask for a private channel, without sharing details publicly.

Please include the commit you tested, the Go version, a minimal reproduction and the impact you expect.
**Use generated values, never real personal data**, in reports, reproductions and test cases (the test
files show how fixtures are generated).

Reports are handled on a best-effort basis by a single maintainer; there is no response-time guarantee.
Fixes are coordinated with the reporter before details are published, and reporters are credited if they
wish.

## Supported versions

The project is pre-alpha and has no releases yet. Only the latest `main` branch is supported.

## In scope

- Personal data escaping a documented guarantee: a value that the [specification](docs/spec/v0.1.md)
  says is detected (shape, boundaries, policy) but is returned unredacted, or an operation that returns
  the unredacted input on failure.
- Cross-session restoration: a session restoring a token it did not issue, including another session's.
- Forged-token disclosure: text that was never issued by a session being restored into a value.
- Token collision disclosure: two different values sharing a token, or a value restored as another one.
- Pre-existing literal disclosure: token-like text that reached `Session.Redact` before the session issued
  it becoming restorable.
- Unbounded memory or CPU: input that makes detection super-linear, makes `RestoreWriter` hold more than
  one token, or grows a session beyond its capacity.
- A panic on crafted input in any exported function.
- Personal data appearing in errors, `String`/`GoString` output, or other diagnostics produced by the
  package.
- Data races in `Redactor` or `Session` (which are documented as safe for concurrent use).

## Out of scope

- The limits the specification documents: forms that are not recognized (spaced digits, full-width
  digits, invisible characters, numbers in words), unformatted values without a keyword under the
  `Balanced` policy, false positives under `Strict`, entities that are not implemented. Please report
  these as regular issues; they are detection-quality improvements.
- What the threat model (specification, §16) excludes: deliberate exfiltration by a malicious user,
  personal data in images, PDFs or audio, names and addresses, prompt injection in general (a model
  repeating the tokens it saw), a compromised process, memory inspection and side channels.
- Misuse that the documentation warns against, such as sharing one session across users.
- Vulnerabilities in the Go toolchain or standard library; report those to the Go project.
