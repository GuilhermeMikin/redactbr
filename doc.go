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
// Valid check digits alone are weak evidence for unformatted numbers: about one in a hundred arbitrary
// 11-digit numbers has valid CPF check digits. Under the default Balanced policy, unformatted identifiers
// are redacted only when the entity's keyword ("cpf" or "cnpj", in any case) appears in the 32 bytes
// before them; formatted identifiers are redacted whenever their check digits are valid. Strict redacts
// every valid candidate, accepting more false positives.
//
// # Security
//
// Matches carry offsets, never values, and no error or panic message produced by this package contains
// detected data. Redaction of a string cannot fail; operations that can fail return an error and an empty
// result, never the unredacted input.
package redactbr
