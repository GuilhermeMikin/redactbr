package redactbr_test

import (
	"strings"
	"testing"

	"github.com/GuilhermeMikin/redactbr"
)

// Fixtures in this file are generated from explainable bases (sequential digits, repeated pairs, letter
// patterns) with the official check-digit algorithms; none was collected. A generated number can still
// coincide with an assigned one.
//
//	CPF   123.456.789-09  987.654.321-00  111.111.111-11 (repeated digits)
//	CNPJ  11.223.344/5566-13 (numeric)  AB.12C.D34/EF56-02 (alphanumeric)

func mustNew(t testing.TB, opts ...redactbr.Option) *redactbr.Redactor {
	t.Helper()
	r, err := redactbr.New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

type redactCase struct {
	name     string
	in       string
	balanced string // expected output under Balanced
	strict   string // expected output under Strict; empty means the same as balanced
}

func runRedactCases(t *testing.T, cases []redactCase) {
	t.Helper()
	balanced := mustNew(t)
	strict := mustNew(t, redactbr.WithPolicy(redactbr.Strict))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := balanced.Redact(tc.in); got != tc.balanced {
				t.Errorf("Balanced:\n got  %q\n want %q", got, tc.balanced)
			}
			want := tc.strict
			if want == "" {
				want = tc.balanced
			}
			if got := strict.Redact(tc.in); got != want {
				t.Errorf("Strict:\n got  %q\n want %q", got, want)
			}
		})
	}
}

func TestRedactCPF(t *testing.T) {
	runRedactCases(t, []redactCase{
		{"formatted with keyword", "CPF 123.456.789-09.", "CPF <CPF>.", ""},
		{"formatted without keyword", "Customer 987.654.321-00 called", "Customer <CPF> called", ""},
		{"formatted with invalid check digits", "CPF 123.456.789-08", "CPF 123.456.789-08", ""},
		{"unformatted without keyword", "order 12345678909 shipped", "order 12345678909 shipped", "order <CPF> shipped"},
		{"unformatted with keyword", "cpf=12345678909", "cpf=<CPF>", ""},
		{"unformatted with keyword in another case", "Cpf: 12345678909", "Cpf: <CPF>", ""},
		{"unformatted with keyword in a JSON key", `{"customerCpf":"12345678909"}`, `{"customerCpf":"<CPF>"}`, ""},
		{"unformatted with invalid check digits", "CPF 12345678908", "CPF 12345678908", ""},
		{"dash-only with keyword", "CPF 123456789-09", "CPF <CPF>", ""},
		{"dash-only without keyword", "ref 123456789-09", "ref 123456789-09", "ref <CPF>"},
		{"cnpj keyword does not count for CPF", "CNPJ 12345678909", "CNPJ 12345678909", "CNPJ <CPF>"},
		{"repeated digits pass the official routine", "CPF 111.111.111-11", "CPF <CPF>", ""},
		{"all zeros", "CPF 000.000.000-00", "CPF 000.000.000-00", ""},
		{"two values separated by a comma", "123.456.789-09,987.654.321-00", "<CPF>,<CPF>", ""},
		{"inside a longer number", "cpf 123456789091", "cpf 123456789091", ""},
		{"formatted inside a longer number", "cpf 123.456.789-091", "cpf 123.456.789-091", ""},
		{"letter before", "cpf x12345678909", "cpf x12345678909", ""},
		{"letter after", "cpf 12345678909x", "cpf 12345678909x", ""},
		{"accented letter before", "cpfé12345678909", "cpfé12345678909", ""},
		{"underscore before", "cpf_12345678909", "cpf_<CPF>", ""},
		{"hyphen before", "id-12345678909", "id-12345678909", "id-<CPF>"},
		{"decimal fraction", "cpf 0.12345678909", "cpf 0.12345678909", ""},
		{"decimal with integer part", "cpf 12345678909.5", "cpf 12345678909.5", ""},
		{"dotted number", "1.123.456.789-09", "1.123.456.789-09", ""},
		{"sentence end", "Meu CPF é 12345678909.", "Meu CPF é <CPF>.", ""},
		{"non-breaking space after keyword", "CPF:\u00a012345678909", "CPF:\u00a0<CPF>", ""},
		{"invalid UTF-8 before", "\xff123.456.789-09", "\xff<CPF>", ""},
		{"partially dotted", "CPF 123.456789-09", "CPF 123.456789-09", ""},
		{"dots instead of hyphen", "CPF 123.456.789.09", "CPF 123.456.789.09", ""},
		{"hyphens instead of dots", "CPF 123-456-789-09", "CPF 123-456-789-09", ""},
		{"spaces instead of separators", "CPF 123 456 789 09", "CPF 123 456 789 09", ""},
		{"full-width digits are not normalized", "CPF １２３.４５６.７８９-０９", "CPF １２３.４５６.７８９-０９", ""},
		{"zero-width space inside", "CPF 123.456\u200b.789-09", "CPF 123.456\u200b.789-09", ""},
		{"ordinal indicator before", "CPF nº123.456.789-09", "CPF nº<CPF>", ""},
		{"feminine ordinal indicator before", "inscrita sob a ª12345678909 cpf", "inscrita sob a ª12345678909 cpf", "inscrita sob a ª<CPF> cpf"},
		{"dotted keyword", "C.P.F.: 12345678909", "C.P.F.: <CPF>", ""},
		{"keyword glued to the value is not a boundary", "CPF123.456.789-09", "CPF123.456.789-09", ""},
		{"combining mark before", "cpf e\u030112345678909", "cpf e\u030112345678909", ""},
		{"combining mark after", "cpf 12345678909\u0301", "cpf 12345678909\u0301", ""},
	})
}

func TestRedactCNPJ(t *testing.T) {
	runRedactCases(t, []redactCase{
		{"numeric formatted", "CNPJ 11.223.344/5566-13", "CNPJ <CNPJ>", ""},
		{"numeric formatted without keyword", "supplier 11.223.344/5566-13", "supplier <CNPJ>", ""},
		{"numeric without dots", "11223344/5566-13", "<CNPJ>", ""},
		{"numeric unformatted without keyword", "id 11223344556613", "id 11223344556613", "id <CNPJ>"},
		{"numeric unformatted with keyword", "cnpj: 11223344556613", "cnpj: <CNPJ>", ""},
		{"combined label", "CPF/CNPJ: 11223344556613", "CPF/CNPJ: <CNPJ>", ""},
		{"cpf keyword does not count for CNPJ", "cpf 11223344556613", "cpf 11223344556613", "cpf <CNPJ>"},
		{"alphanumeric formatted", "CNPJ AB.12C.D34/EF56-02", "CNPJ <CNPJ>", ""},
		{"alphanumeric lowercase formatted", "cnpj ab.12c.d34/ef56-02", "cnpj <CNPJ>", ""},
		{"alphanumeric mixed case formatted", "Ab.12C.d34/Ef56-02", "<CNPJ>", ""},
		{"alphanumeric without dots", "AB12CD34/EF56-02", "<CNPJ>", ""},
		{"alphanumeric unformatted without keyword", "code AB12CD34EF5602", "code AB12CD34EF5602", "code <CNPJ>"},
		{"alphanumeric unformatted with keyword", "CNPJ AB12CD34EF5602", "CNPJ <CNPJ>", ""},
		{"alphanumeric lowercase unformatted with keyword", "cnpj ab12cd34ef5602", "cnpj <CNPJ>", ""},
		{"invalid check digits", "AB.12C.D34/EF56-03", "AB.12C.D34/EF56-03", ""},
		{"letter as check digit", "AB.12C.D34/EF56-0A", "AB.12C.D34/EF56-0A", ""},
		{"all zeros", "00.000.000/0000-00", "00.000.000/0000-00", ""},
		{"official FAQ example with mismatched check digits", "CNPJ AA345678/0003-29", "CNPJ AA345678/0003-29", ""},
		{"official FAQ example", "CNPJ 12.ABC.345/01DE-35", "CNPJ <CNPJ>", ""},
		{"letter before", "cnpj XAB12CD34EF5602", "cnpj XAB12CD34EF5602", ""},
		{"digit after", "cnpj 112233445566130", "cnpj 112233445566130", ""},
		{"in a URL path", "GET /suppliers/11223344556613/orders", "GET /suppliers/11223344556613/orders", "GET /suppliers/<CNPJ>/orders"},
		{"dotted keyword", "C.N.P.J. 11223344556613", "C.N.P.J. <CNPJ>", ""},
		{"after a detected value and a dot", "12345678909.AB12CD34EF5602", "12345678909.AB12CD34EF5602", "<CPF>.<CNPJ>"},
		{"without dots, lowercase, no keyword", "ab12cd34/ef56-02", "ab12cd34/ef56-02", "<CNPJ>"},
		{"without dots, lowercase, keyword", "cnpj ab12cd34/ef56-02", "cnpj <CNPJ>", ""},
		// Paths shaped like XXXXXXXX/XXXX-DD whose check digits happen to be valid.
		{"URL path with lowercase letters", "GET /builds/20261030/main-12/log", "GET /builds/20261030/main-12/log", "GET /builds/<CNPJ>/log"},
		{"another URL path", "a1b2c3d4/runs-63 and checkout/step-19", "a1b2c3d4/runs-63 and checkout/step-19", "<CNPJ> and <CNPJ>"},
	})
}

func TestRedactEmail(t *testing.T) {
	runRedactCases(t, []redactCase{
		{"simple", "write to ana.silva@example.com.", "write to <EMAIL>.", ""},
		{"plus addressing and subdomains", "a+b@mail.example.com.br", "<EMAIL>", ""},
		{"uppercase", "ANA@EXAMPLE.COM", "<EMAIL>", ""},
		{"in angle brackets", "Ana <ana@example.com>", "Ana <<EMAIL>>", ""},
		{"leading dots are punctuation", "see:...ana@example.com", "see:...<EMAIL>", ""},
		{"accented local part", "joão@example.com", "<EMAIL>", ""},
		{"punycode top-level domain", "ana@example.xn--p1ai", "<EMAIL>", ""},
		{"no top-level domain", "ana@localhost", "ana@localhost", ""},
		{"numeric top-level domain", "ana@192.0.2.1", "ana@192.0.2.1", ""},
		{"IP address literal", "ana@[192.0.2.1]", "ana@[192.0.2.1]", ""},
		{"empty label", "ana@example..com", "ana@example..com", ""},
		{"local part longer than 64 octets", strings.Repeat("a", 65) + "@example.com", strings.Repeat("a", 65) + "@example.com", ""},
		{"local part of exactly 64 octets", strings.Repeat("a", 64) + "@example.com", "<EMAIL>", ""},
		{"digits of a CPF as local part", "12345678909@example.com", "<EMAIL>", ""},
		{"second @ right after an address", "ana@example.com@example.org", "<EMAIL>@example.org", ""},
		{"hyphenated text glued after", "Contact ana@example.com-thanks", "Contact <EMAIL>-thanks", ""},
		{"number glued after", "session:ana@example.com-1730303030", "session:<EMAIL>-1730303030", ""},
		{"dot and digit glued after", "ana@example.com.2", "<EMAIL>.2", ""},
		{"hyphenated domain", "ana@my-company.example.com", "<EMAIL>", ""},
		{"long trailing punctuation", "ana@example.com" + strings.Repeat(".", 260), "<EMAIL>" + strings.Repeat(".", 260), ""},
		{"long trailing hyphens", "ana@example.com" + strings.Repeat("-", 260), "<EMAIL>" + strings.Repeat("-", 260), ""},
		{"long leading punctuation", strings.Repeat(".", 70) + "ana@example.com", strings.Repeat(".", 70) + "<EMAIL>", ""},
		{"uppercase punycode", "ana@example.XN--P1AI", "<EMAIL>", ""},
		{"apostrophe in the local part", "sant'anna@example.com", "<EMAIL>", ""},
		{"quoted address", "'ana@example.com'", "'<EMAIL>'", ""},
		{"decomposed accent", "joa\u0303o@example.com", "<EMAIL>", ""},
		{"two addresses glued with a hyphen", "ana@example.com-bob@example.org", "<EMAIL><EMAIL>", ""},
		{"address followed by digits with CPF check digits", "a@b.co.12345678909", "<EMAIL>.12345678909", "<EMAIL>.<CPF>"},
	})
}

func TestRedactMixed(t *testing.T) {
	r := mustNew(t)
	in := "CPF 123.456.789-09, CNPJ AB.12C.D34/EF56-02, email ana@example.com, cpf 98765432100"
	want := "CPF <CPF>, CNPJ <CNPJ>, email <EMAIL>, cpf <CPF>"
	if got := r.Redact(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestContextWindow(t *testing.T) {
	r := mustNew(t)
	// The keyword must lie entirely within the 32 bytes before the value.
	inside := "cpf" + strings.Repeat(" ", 29) + "12345678909"
	if got := r.Redact(inside); got == inside {
		t.Error("keyword 32 bytes before the value was not used as context")
	}
	outside := "cpf" + strings.Repeat(" ", 30) + "12345678909"
	if got := r.Redact(outside); got != outside {
		t.Error("keyword 33 bytes before the value was used as context")
	}
	after := "12345678909 is my cpf"
	if got := r.Redact(after); got != after {
		t.Error("a keyword after the value was used as context")
	}
}

func TestFind(t *testing.T) {
	r := mustNew(t)
	text := "CPF 123.456.789-09 / ana@example.com"
	got := r.Find(text)
	want := []redactbr.Match{
		{Entity: redactbr.CPF, Start: 4, End: 18},
		{Entity: redactbr.Email, Start: 21, End: 36},
	}
	if len(got) != len(want) {
		t.Fatalf("Find returned %d matches, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %v, want %v", i, got[i], want[i])
		}
	}
	if s := got[0].String(); s != "CPF[4:18]" {
		t.Errorf("Match.String() = %q, want %q", s, "CPF[4:18]")
	}
	if len(r.Find("nothing to see here")) != 0 {
		t.Error("Find reported a match in text without personal data")
	}
}

func TestWithEntities(t *testing.T) {
	r := mustNew(t, redactbr.WithEntities(redactbr.CPF))
	in := "123.456.789-09 11.223.344/5566-13 ana@example.com"
	want := "<CPF> 11.223.344/5566-13 ana@example.com"
	if got := r.Redact(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	r = mustNew(t, redactbr.WithEntities(redactbr.Email, redactbr.CNPJ, redactbr.Email))
	want = "123.456.789-09 <CNPJ> <EMAIL>"
	if got := r.Redact(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		opts []redactbr.Option
	}{
		{"no entities", []redactbr.Option{redactbr.WithEntities()}},
		{"unknown entity", []redactbr.Option{redactbr.WithEntities(redactbr.CPF, "PHONE")}},
		{"unknown policy", []redactbr.Option{redactbr.WithPolicy(redactbr.Policy(7))}},
		{"zero session capacity", []redactbr.Option{redactbr.WithMaxSessionValues(0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := redactbr.New(tt.opts...)
			if err == nil || r != nil {
				t.Fatalf("New returned (%v, %v), want a nil Redactor and an error", r, err)
			}
			if !strings.HasPrefix(err.Error(), "redactbr: ") {
				t.Errorf("error %q lacks the package prefix", err)
			}
		})
	}
}

func TestZeroRedactorDetectsEverything(t *testing.T) {
	var r redactbr.Redactor // not created with New: must not silently detect nothing
	in := "CPF 123.456.789-09, CNPJ AB.12C.D34/EF56-02, ana@example.com, cpf 98765432100"
	want := mustNew(t).Redact(in)
	if got := r.Redact(in); got != want || got == in {
		t.Fatalf("zero Redactor: got %q, want %q", got, want)
	}
	s := r.NewSession()
	out, err := s.Redact(in)
	if err != nil || out == in || s.Restore(out) != in {
		t.Fatalf("zero Redactor session: Redact returned (%d bytes, %v)", len(out), err)
	}
}

func TestPolicyString(t *testing.T) {
	for p, want := range map[redactbr.Policy]string{
		redactbr.Balanced:  "balanced",
		redactbr.Strict:    "strict",
		redactbr.Policy(9): "Policy(9)",
	} {
		if got := p.String(); got != want {
			t.Errorf("Policy(%d).String() = %q, want %q", int(p), got, want)
		}
	}
}
