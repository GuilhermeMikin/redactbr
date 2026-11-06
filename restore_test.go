package redactbr

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

var _ io.WriteCloser = (*RestoreWriter)(nil)

// streamSession returns a session with two known tokens: <CPF_k7q2xm9pb3> for 123.456.789-09 and
// <EMAIL_0000000000> for ana@example.com.
func streamSession(t testing.TB) *Session {
	t.Helper()
	s := newTestRedactor(t).NewSession()
	s.fill = idSource("k7q2xm9pb3", "0000000000")
	if out, err := s.Redact("123.456.789-09 ana@example.com"); err != nil || out != "<CPF_k7q2xm9pb3> <EMAIL_0000000000>" {
		t.Fatalf("setup: Redact = (%d bytes, %v)", len(out), err)
	}
	return s
}

const streamText = "Customer <CPF_k7q2xm9pb3> wrote from <EMAIL_0000000000>.<CPF_k7q2xm9pb3><EMAIL_0000000000> " +
	"a < b, <CPF_0000000000> is forged, <<CPF_k7q2xm9pb3>>, <CPF>, <EMAIL_000000000 and the end <CPF_k7q2"

func writeChunks(t *testing.T, s *Session, chunks ...string) string {
	t.Helper()
	var out bytes.Buffer
	w := s.RestoreWriter(&out)
	for _, c := range chunks {
		if n, err := w.Write([]byte(c)); err != nil || n != len(c) {
			t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(c))
		}
		if w.held >= maxTokenLen {
			t.Fatalf("held back %d bytes, more than one token", w.held)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return out.String()
}

func TestRestoreWriterEverySplit(t *testing.T) {
	s := streamSession(t)
	want := s.Restore(streamText)
	if !strings.Contains(want, "Customer 123.456.789-09 wrote from ana@example.com.123.456.789-09ana@example.com ") {
		t.Fatal("Restore did not restore the known tokens")
	}
	if !strings.HasSuffix(want, "<CPF_0000000000> is forged, <123.456.789-09>, <CPF>, <EMAIL_000000000 and the end <CPF_k7q2") {
		t.Fatal("Restore changed text that is not an issued token")
	}
	for i := 0; i <= len(streamText); i++ {
		for j := i; j <= len(streamText); j += 7 {
			if got := writeChunks(t, s, streamText[:i], streamText[i:j], streamText[j:]); got != want {
				t.Fatalf("split at %d and %d: output differs from Restore", i, j)
			}
		}
	}
}

func TestRestoreWriterOneByteChunks(t *testing.T) {
	s := streamSession(t)
	chunks := strings.Split(streamText, "")
	if got, want := writeChunks(t, s, chunks...), s.Restore(streamText); got != want {
		t.Fatal("1-byte chunks: output differs from Restore")
	}
}

func TestRestoreWriterTokenStartsAtLastByte(t *testing.T) {
	s := streamSession(t)
	if got := writeChunks(t, s, "Customer <", "CPF_k7q2xm9pb3> ok"); got != "Customer 123.456.789-09 ok" {
		t.Errorf("got %d bytes, want the restored text", len(got))
	}
	if got := writeChunks(t, s, "Customer <CPF_k7q2xm9pb3", "> ok"); got != "Customer 123.456.789-09 ok" {
		t.Errorf("got %d bytes, want the restored text", len(got))
	}
}

func TestRestoreWriterUnfinishedTokenAtEOF(t *testing.T) {
	s := streamSession(t)
	if got := writeChunks(t, s, "the end <CPF_k7q2xm9p"); got != "the end <CPF_k7q2xm9p" {
		t.Errorf("an unfinished token at EOF was not written verbatim")
	}
}

func TestRestoreWriterBoundsHeldBytes(t *testing.T) {
	s := streamSession(t)
	var out bytes.Buffer
	w := s.RestoreWriter(&out)
	// An attacker-controlled stream of token-like prefixes never makes the writer hold more than one token.
	long := "<CPF_" + strings.Repeat("0123456789", 1000) + strings.Repeat("<", 1000) + strings.Repeat("<EMAIL_", 500)
	for i := range len(long) {
		if _, err := w.Write([]byte{long[i]}); err != nil {
			t.Fatal(err)
		}
		if w.held >= maxTokenLen {
			t.Fatalf("held back %d bytes after %d bytes written", w.held, i+1)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if out.String() != long {
		t.Error("text without issued tokens was changed")
	}
	// A partial token is released as soon as it cannot be a token any more.
	out.Reset()
	w = s.RestoreWriter(&out)
	if _, err := w.Write([]byte("x <CPF_k7q2")); err != nil {
		t.Fatal(err)
	}
	if out.String() != "x " || w.held != len("<CPF_k7q2") {
		t.Fatalf("after a partial token: wrote %q, held %d", out.String(), w.held)
	}
	if _, err := w.Write([]byte("!")); err != nil {
		t.Fatal(err)
	}
	if out.String() != "x <CPF_k7q2!" || w.held != 0 {
		t.Fatalf("after breaking the token: wrote %q, held %d", out.String(), w.held)
	}
}

func TestRestoreWriterRestoresTokensIssuedLater(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	var out bytes.Buffer
	w := s.RestoreWriter(&out)
	tok := mustRedact(t, s, "987.654.321-00")
	if _, err := w.Write([]byte("value: " + tok)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "value: 987.654.321-00" {
		t.Error("a token issued after the writer was created was not restored")
	}
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestRestoreWriterErrors(t *testing.T) {
	s := streamSession(t)
	boom := errors.New("disk full")
	w := s.RestoreWriter(failingWriter{boom})
	if _, err := w.Write([]byte("hello")); !errors.Is(err, boom) {
		t.Fatalf("Write returned %v, want the underlying error", err)
	}
	if _, err := w.Write([]byte("again")); !errors.Is(err, boom) {
		t.Errorf("the underlying error is not sticky: %v", err)
	}
	if err := w.Close(); !errors.Is(err, boom) {
		t.Errorf("Close returned %v, want the underlying error", err)
	}

	var out bytes.Buffer
	w = s.RestoreWriter(&out)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("late")); !errors.Is(err, errWriterClosed) {
		t.Errorf("Write after Close returned %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("a second Close returned %v", err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestRestoreWriterReportsShortWrites(t *testing.T) {
	w := streamSession(t).RestoreWriter(shortWriter{})
	if _, err := w.Write([]byte("hello")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write returned %v, want io.ErrShortWrite", err)
	}
	if _, err := w.Write([]byte("again")); !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("a short write is not sticky: %v", err)
	}
}

func TestRestoreWriterLeavesReservedAndForeignTokens(t *testing.T) {
	s := streamSession(t)
	foreign := mustRedact(t, newTestRedactor(t).NewSession(), "987.654.321-00")
	mustRedact(t, s, "<CPF_aaaaaaaaaa>")
	in := "<CPF_aaaaaaaaaa> " + foreign + " <CPF_k7q2xm9pb3>"
	want := "<CPF_aaaaaaaaaa> " + foreign + " 123.456.789-09"
	for size := 1; size <= len(in); size++ {
		var chunks []string
		for i := 0; i < len(in); i += size {
			chunks = append(chunks, in[i:min(i+size, len(in))])
		}
		if got := writeChunks(t, s, chunks...); got != want {
			t.Fatalf("chunks of %d bytes: got %q, want %q", size, got, want)
		}
	}
}

func TestRestoreWriterStringHidesContents(t *testing.T) {
	s := streamSession(t)
	var out bytes.Buffer
	w := s.RestoreWriter(&out)
	if _, err := w.Write([]byte("<CPF_k7q2xm9pb3> <CPF_k7q2")); err != nil {
		t.Fatal(err)
	}
	for _, got := range []string{
		fmt.Sprint(w), fmt.Sprintf("%+v", w), fmt.Sprintf("%#v", w),
		fmt.Sprint(*w), fmt.Sprintf("%+v", *w), fmt.Sprintf("%#v", *w), // dereferenced
		fmt.Sprintf("%+v", struct{ W *RestoreWriter }{w}), fmt.Sprintf("%+v", struct{ w RestoreWriter }{*w}), // in a struct
	} {
		// The held bytes are "<CPF_k7q2"; as numbers, %+v would print 60 67 80 70.
		if strings.Contains(got, "123") || strings.Contains(got, "k7q2") || strings.Contains(got, "60 67") {
			t.Errorf("formatting a RestoreWriter revealed buffered text: %s", got)
		}
	}
	if strings.Contains(string(w.out[:cap(w.out)]), "123.456.789-09") {
		t.Error("the reusable output buffer still holds a restored value")
	}
}

// FuzzRestoreWriter checks that streaming restoration with arbitrary chunking equals Restore on the whole
// text, and never holds back more than one token.
func FuzzRestoreWriter(f *testing.F) {
	f.Add(streamText, uint8(3))
	f.Add("<CPF_k7q2xm9pb3>", uint8(1))
	f.Add("<<<EMAIL_0000000000>>>", uint8(2))
	f.Add("<CPF_k7q2xm9pb", uint8(5))
	f.Fuzz(func(t *testing.T, text string, step uint8) {
		s := streamSession(t)
		size := int(step%32) + 1
		var chunks []string
		for i := 0; i < len(text); i += size {
			chunks = append(chunks, text[i:min(i+size, len(text))])
		}
		if got, want := writeChunks(t, s, chunks...), s.Restore(text); got != want {
			t.Fatalf("chunks of %d bytes: output differs from Restore", size)
		}
	})
}

// FuzzSessionRoundTrip checks the session invariants on arbitrary input: restoring the redacted text
// gives back the input (with each value in the form first seen), another session restores nothing, and
// token-like text from the input never becomes restorable. The random source cycles through four ids, so
// issued ids collide with look-alikes in the input often and the reservation logic is exercised.
func FuzzSessionRoundTrip(f *testing.F) {
	for _, s := range []string{
		"CPF 123.456.789-09 / cpf 12345678909 / ana@example.com / ANA@EXAMPLE.COM",
		"<CPF_aaaaaaaaaa> and 123.456.789-09", "<CPF_bbbbbbbbbb> 987.654.321-00 <CPF_aaaaaaaaaa>",
		"cnpj ab12cd34ef5602 AB.12C.D34/EF56-02", "<EMAIL_aaaaaaaaaa> ana@example.com",
	} {
		f.Add(s)
	}
	r := newTestRedactor(f)
	f.Fuzz(func(t *testing.T, in string) {
		s := r.NewSession()
		s.fill = lowEntropySource()
		out, err := s.Redact(in)
		if errors.Is(err, ErrSessionFull) || errors.Is(err, errTokenCollision) {
			t.Skip("input larger than the capacity or than the four test ids")
		}
		if err != nil {
			t.Fatal(err)
		}
		var want strings.Builder
		first := make(map[valueKey]string)
		last := 0
		for _, m := range r.Find(in) {
			v := in[m.Start:m.End]
			k := valueKey{m.Entity, normalize(m.Entity, v)}
			if _, ok := first[k]; !ok {
				first[k] = v
			}
			want.WriteString(in[last:m.Start] + first[k])
			last = m.End
		}
		want.WriteString(in[last:])
		if s.Restore(out) != want.String() {
			t.Fatal("Restore(Redact(x)) differs from x with each value in its first-seen form")
		}
		if r.NewSession().Restore(out) != out {
			t.Fatal("another session restored a token")
		}
		for _, lit := range tokenLiterals(in) {
			if _, ok := s.values[lit]; ok {
				t.Fatal("token-like text from the input became restorable")
			}
		}
	})
}
