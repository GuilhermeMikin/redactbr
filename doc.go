// Package redactbr detects and redacts Brazilian personal data — CPF, numeric and alphanumeric CNPJ — and
// email addresses in text, in-process, using only the standard library.
//
// A Redactor holds the configuration and is safe for concurrent use:
//
//	r, err := redactbr.New()
//	if err != nil {
//		// invalid configuration
//	}
//	out := r.Redact("Customer CPF: 123.456.789-09")
//	// out == "Customer CPF: <CPF>"
//
// # What is detected
//
// CPF is recognized as DDD.DDD.DDD-DD (formatted), DDDDDDDDD-DD or DDDDDDDDDDD (unformatted). CNPJ is
// recognized as XX.XXX.XXX/XXXX-DD or XXXXXXXX/XXXX-DD (formatted) or XXXXXXXXXXXXDD (unformatted), where
// X is a digit or a letter (the alphanumeric CNPJ, in either case) and D a digit. Every candidate must have
// valid check digits and must not touch a letter or digit on either side. Email addresses are recognized
// by structure. The full contract, with sources and limitations, is in docs/spec/v0.1.md.
//
// # Policies
//
// Valid check digits alone are weak evidence for unformatted numbers: one in a hundred arbitrary 11-digit
// numbers has valid CPF check digits. Under the default Balanced policy, unformatted identifiers are
// redacted only when the entity's keyword ("cpf" or "cnpj", in any case) appears in the 32 bytes before
// them; formatted identifiers are redacted whenever their check digits are valid. Strict redacts every
// valid candidate, accepting more false positives.
//
// # Reversible sessions
//
// A Session replaces values with tokens that an LLM can copy, and restores them in its answer:
//
//	session := r.NewSession()
//	prompt, err := session.Redact("My CPF is 123.456.789-09")
//	if err != nil {
//		// fail closed: do not send anything
//	}
//	// prompt == "My CPF is <CPF_k7q2xm9pb3>" (the id is random)
//	answer := session.Restore(modelAnswer)
//
// The same value keeps the same token for the lifetime of the session. Only tokens the session issued
// are restored; anything else, including tokens of other sessions and modified tokens, is left as it is.
// Token-like text that reaches Session.Redact before the session issued it is reserved and never issued,
// so a user who writes something that looks like a token cannot make it restorable. A session is a trust
// scope: use one per conversation, never one shared across users.
//
// # Streaming
//
// Session.RestoreWriter restores tokens in a stream, such as an LLM response arriving in chunks, even when
// a token is split across chunks. It holds back at most 17 bytes between writes:
//
//	w := session.RestoreWriter(dst)
//	for chunk := range chunks {
//		if _, err := w.Write(chunk); err != nil {
//			// handle the error
//		}
//	}
//	err := w.Close() // writes any held-back bytes
//
// # Concurrency and errors
//
// Redactor and Session are safe for concurrent use; a RestoreWriter is not. Redaction of a string by a
// Redactor cannot fail. Session.Redact can fail (for example with ErrSessionFull); it then returns an
// empty string and an error, never the unredacted input. Matches carry offsets, never values, and no error
// or panic message produced by this package contains detected data.
package redactbr
