package redactbr_test

import (
	"io"
	"strings"
	"testing"

	"github.com/GuilhermeMikin/redactbr"
)

// Benchmark inputs are built from fixed, generated lines so results are comparable between runs. They
// measure this package only; they are not a comparison with other tools.

const (
	piiLine   = "Customer 123.456.789-09 asked about invoice 4521 for CNPJ AB.12C.D34/EF56-02; reply to ana.silva@example.com. "
	proseLine = "Order 4521 shipped on 2026-10-30 with tracking code 77812, total R$ 1.234,56, delivery in 3 days. "
)

// mixedText alternates a line with three values and a line without any until it reaches size bytes.
func mixedText(size int) string {
	var b strings.Builder
	for i := 0; b.Len() < size; i++ {
		if i%2 == 0 {
			b.WriteString(piiLine)
		} else {
			b.WriteString(proseLine)
		}
	}
	return b.String()
}

func benchRedact(b *testing.B, r *redactbr.Redactor, text string) {
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		_ = r.Redact(text)
	}
}

func BenchmarkRedactCPF(b *testing.B) {
	benchRedact(b, mustNew(b), "Customer CPF: 123.456.789-09, thanks.")
}

func BenchmarkRedactCNPJ(b *testing.B) {
	benchRedact(b, mustNew(b), "Supplier CNPJ AB.12C.D34/EF56-02 confirmed.")
}

func BenchmarkRedactNoPII10KB(b *testing.B) {
	benchRedact(b, mustNew(b), strings.Repeat(proseLine, 10*1024/len(proseLine)+1))
}

func BenchmarkMixed1KB(b *testing.B) {
	benchRedact(b, mustNew(b), mixedText(1024))
}

func BenchmarkMixed10KB(b *testing.B) {
	benchRedact(b, mustNew(b), mixedText(10*1024))
}

func BenchmarkMixed10KBStrict(b *testing.B) {
	benchRedact(b, mustNew(b, redactbr.WithPolicy(redactbr.Strict)), mixedText(10*1024))
}

// BenchmarkTokenize measures a first Session.Redact: every value is new and gets a random token.
func BenchmarkTokenize(b *testing.B) {
	r := mustNew(b)
	text := mixedText(1024)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := r.NewSession().Redact(text); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTokenizeKnownValues measures Session.Redact when every value already has a token, as in later
// turns of a conversation.
func BenchmarkTokenizeKnownValues(b *testing.B) {
	s := mustNew(b).NewSession()
	text := mixedText(1024)
	if _, err := s.Redact(text); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Redact(text); err != nil {
			b.Fatal(err)
		}
	}
}

func tokenizedAnswer(b *testing.B, size int) (*redactbr.Session, string) {
	b.Helper()
	s := mustNew(b).NewSession()
	answer, err := s.Redact(mixedText(size))
	if err != nil {
		b.Fatal(err)
	}
	return s, answer
}

func BenchmarkRestoreToken(b *testing.B) {
	s, answer := tokenizedAnswer(b, 1024)
	b.SetBytes(int64(len(answer)))
	b.ReportAllocs()
	for b.Loop() {
		_ = s.Restore(answer)
	}
}

// BenchmarkRestoreStreaming restores a 10 KB answer written in 16-byte chunks, so that many tokens are
// split across writes.
func BenchmarkRestoreStreaming(b *testing.B) {
	s, answer := tokenizedAnswer(b, 10*1024)
	data := []byte(answer)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		w := s.RestoreWriter(io.Discard)
		for i := 0; i < len(data); i += 16 {
			if _, err := w.Write(data[i:min(i+16, len(data))]); err != nil {
				b.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
