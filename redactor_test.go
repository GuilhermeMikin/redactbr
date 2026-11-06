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
//	CPF   123.456.789-09  987.654.321-00  314.159.265-90  111.111.111-11 (repeated digits)
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
		{"feminine ordinal indicator before", "inscrita sob a ª12345678909", "inscrita sob a ª12345678909", "inscrita sob a ª<CPF>"},
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
	// The same holds for the 32 bytes after the value.
	inside = "12345678909" + strings.Repeat(" ", 29) + "cpf"
	if got := r.Redact(inside); got == inside {
		t.Error("keyword 32 bytes after the value was not used as context")
	}
	outside = "12345678909" + strings.Repeat(" ", 30) + "cpf"
	if got := r.Redact(outside); got != outside {
		t.Error("keyword 33 bytes after the value was used as context")
	}
}

func TestKeywordAfterValue(t *testing.T) {
	runRedactCases(t, []redactCase{
		// The keyword is about the value before it.
		{"sentence", "12345678909 é meu CPF", "<CPF> é meu CPF", ""},
		{"sentence in English, with a period", "12345678909 is my cpf.", "<CPF> is my cpf.", ""},
		{"parentheses", "12345678909 (CPF)", "<CPF> (CPF)", ""},
		{"dash and more text", "12345678909 - CPF - Ana", "<CPF> - CPF - Ana", ""},
		{"hyphen", "12345678909-cpf", "<CPF>-cpf", ""},
		{"after a comma", "12345678909, meu CPF", "<CPF>, meu CPF", ""},
		{"dotted keyword", "12345678909 é meu C.P.F.", "<CPF> é meu C.P.F.", ""},
		{"plural", "12345678909 consta nos CPFs", "<CPF> consta nos CPFs", ""},
		{"quoted value", `"12345678909" é o CPF`, `"<CPF>" é o CPF`, ""},
		{"value of the next JSON field", `{"doc":"12345678909","tipo":"CPF"}`, `{"doc":"<CPF>","tipo":"CPF"}`, ""},
		{"CSV row", "12345678909,CPF,Ana", "<CPF>,CPF,Ana", ""},
		{"table row", "| 12345678909 | CPF |", "| <CPF> | CPF |", ""},
		{"tab-separated", "12345678909\tCPF", "<CPF>\tCPF", ""},
		{"closing tag", "12345678909</cpf>", "<CPF></cpf>", ""},
		{"partially formatted", "123456789-09 é meu CPF", "<CPF> é meu CPF", ""},
		{"numeric CNPJ", "11223344556613 é o CNPJ", "<CNPJ> é o CNPJ", ""},
		{"alphanumeric CNPJ", "AB12CD34EF5602 é o cnpj", "<CNPJ> é o cnpj", ""},
		{"lowercase CNPJ without dots", "ab12cd34/ef56-02 é o CNPJ", "<CNPJ> é o CNPJ", ""},
		{"keyword followed by a number later", "12345678909 é meu CPF e 42 é meu pedido", "<CPF> é meu CPF e 42 é meu pedido", ""},
		{"keyword followed by a closing bracket and a number", "12345678909 (CPF) 42", "<CPF> (CPF) 42", ""},
		// The keyword labels what comes next.
		{"key=value log", "id=12345678909 cpf=98765432100", "id=12345678909 cpf=<CPF>", "id=<CPF> cpf=<CPF>"},
		{"key=value log with another value", "id=12345678909 cpf=ok", "id=12345678909 cpf=ok", "id=<CPF> cpf=ok"},
		{"compact JSON", `{"id":"12345678909","cpf":"98765432100"}`, `{"id":"12345678909","cpf":"<CPF>"}`, `{"id":"<CPF>","cpf":"<CPF>"}`},
		{"spaced JSON", `{"id": 12345678909, "cpf": null}`, `{"id": 12345678909, "cpf": null}`, `{"id": <CPF>, "cpf": null}`},
		{"Python literal", `{'id': '12345678909', 'cpf': 'x'}`, `{'id': '12345678909', 'cpf': 'x'}`, `{'id': '<CPF>', 'cpf': 'x'}`},
		{"followed by a number", "pedido 12345678909 cpf 98765432100", "pedido 12345678909 cpf <CPF>", "pedido <CPF> cpf <CPF>"},
		{"followed by a dash and a number", "pedido 12345678909 CPF - 98765432100", "pedido 12345678909 CPF - <CPF>", "pedido <CPF> CPF - <CPF>"},
		{"followed by a bracket and a number", "pedido 12345678909 CPF (98765432100)", "pedido 12345678909 CPF (<CPF>)", "pedido <CPF> CPF (<CPF>)"},
		{"followed by a hash and a number", "pedido 12345678909 CPF #98765432100", "pedido 12345678909 CPF #<CPF>", "pedido <CPF> CPF #<CPF>"},
		{"followed by a colon", "12345678909 é meu CPF: ok", "12345678909 é meu CPF: ok", "<CPF> é meu CPF: ok"},
		// Something separates the keyword from the value.
		{"another sentence", "pedido 12345678909. CPF do cliente", "pedido 12345678909. CPF do cliente", "pedido <CPF>. CPF do cliente"},
		{"after an exclamation", "12345678909! CPF", "12345678909! CPF", "<CPF>! CPF"},
		{"after a question", "12345678909? CPF", "12345678909? CPF", "<CPF>? CPF"},
		{"after a semicolon", "12345678909; CPF", "12345678909; CPF", "<CPF>; CPF"},
		{"on the next line", "pedido 12345678909\nCPF do cliente", "pedido 12345678909\nCPF do cliente", "pedido <CPF>\nCPF do cliente"},
		{"after a carriage return", "pedido 12345678909\r\nCPF", "pedido 12345678909\r\nCPF", "pedido <CPF>\r\nCPF"},
		{"after a written-out line break", `pedido 12345678909\n CPF`, `pedido 12345678909\n CPF`, `pedido <CPF>\n CPF`},
		{"another number in between", "pedido 12345678909 lote 7 CPF", "pedido 12345678909 lote 7 CPF", "pedido <CPF> lote 7 CPF"},
		// The keyword is part of another word.
		{"key in camel case", "12345678909 customerCpf", "12345678909 customerCpf", "<CPF> customerCpf"},
		{"key in snake case", "12345678909 nr_cpf", "12345678909 nr_cpf", "<CPF> nr_cpf"},
		{"key prefix", "12345678909 cpf_status", "12345678909 cpf_status", "<CPF> cpf_status"},
		{"letters after", "12345678909 cpfoo", "12345678909 cpfoo", "<CPF> cpfoo"},
		{"accented letter before", "12345678909 écpf", "12345678909 écpf", "<CPF> écpf"},
		{"combining mark after", "12345678909 cpf\u0301", "12345678909 cpf\u0301", "<CPF> cpf\u0301"},
		// One entity's keyword does not count for the other.
		{"cnpj keyword after a CPF", "12345678909 é meu CNPJ", "12345678909 é meu CNPJ", "<CPF> é meu CNPJ"},
		{"cpf keyword after a CNPJ", "11223344556613 é meu CPF", "11223344556613 é meu CPF", "<CNPJ> é meu CPF"},
		// Validity comes first.
		{"invalid check digits", "12345678908 é meu CPF", "12345678908 é meu CPF", ""},
	})
}

// TestRuleAfterValueAgainstSymmetricWindow keeps the comparison in decisions log D-026 reproducible: 36 sentences
// whose number has valid check digits, with a keyword that is about the number or not. They show the shapes that
// matter; they are not a measurement on real traffic.
func TestRuleAfterValueAgainstSymmetricWindow(t *testing.T) {
	const v = "12345678909"
	about := []string{
		v + " é meu CPF", v + " is my cpf.", v + " (CPF)", v + " - CPF - Ana", v + ", meu CPF", v + " é meu C.P.F.",
		v + " consta nos CPFs", `"` + v + `" é o CPF`, `{"doc":"` + v + `","tipo":"CPF"}`, v + ",CPF,Ana",
		"| " + v + " | CPF |", v + "\tCPF", v + "</cpf>", v + " é meu CPF e 42 é meu pedido", v + " (CPF) 42",
		v + " é meu CPF: segue",
	}
	notAbout := []string{
		"id=" + v + " cpf=98765432100", "id=" + v + " cpf=ok", `{"id":"` + v + `","cpf":"98765432100"}`,
		`{"id": ` + v + `, "cpf": null}`, `{'id': '` + v + `', 'cpf': 'x'}`, "pedido " + v + " cpf 98765432100",
		"pedido " + v + " CPF - 98765432100", "pedido " + v + " CPF (98765432100)", "pedido " + v + ". CPF do cliente",
		v + "! CPF", v + "; CPF", "pedido " + v + "\nCPF do cliente", "pedido " + v + "\r\nCPF", `pedido ` + v + `\n CPF`,
		"pedido " + v + " lote 7 CPF", v + " customerCpf", v + " nr_cpf", v + " cpf_status", v + " cpfoo",
		"pedido " + v + " do cliente sem CPF",
	}
	r := mustNew(t)
	rule := func(s string) bool { return !strings.Contains(r.Redact(s), v) }
	symmetricWindow := func(s string) bool { // the same substring rule as before a value, without dots
		end := strings.Index(s, v) + len(v)
		return strings.Contains(strings.ToLower(s[end:min(len(s), end+32)]), "cpf")
	}
	count := func(texts []string, accepts func(string) bool) int {
		n := 0
		for _, text := range texts {
			if accepts(text) {
				n++
			}
		}
		return n
	}
	for _, tt := range []struct {
		name            string
		accepts         func(string) bool
		about, notAbout int
	}{{"adopted rule", rule, 15, 1}, {"symmetric window", symmetricWindow, 15, 20}} {
		if a, n := count(about, tt.accepts), count(notAbout, tt.accepts); a != tt.about || n != tt.notAbout {
			t.Errorf("%s accepts %d of %d and %d of %d, want %d and %d", tt.name, a, len(about), n, len(notAbout), tt.about, tt.notAbout)
		}
	}
}

func TestValueLists(t *testing.T) {
	far := strings.Repeat(" ", 33)  // beyond the window
	near := strings.Repeat(" ", 32) // at the edge of the window
	runRedactCases(t, []redactCase{
		{"keyword first, past the window", "CPFs: 12345678909, 98765432100, 31415926590, 12345678909, 98765432100",
			"CPFs: <CPF>, <CPF>, <CPF>, <CPF>, <CPF>", ""},
		{"column under a header", "cpf\n12345678909\n98765432100\n31415926590\n12345678909\n98765432100",
			"cpf\n<CPF>\n<CPF>\n<CPF>\n<CPF>\n<CPF>", ""},
		{"an invalid number does not end a list", "CPFs: 12345678909, 12345678908, 98765432100, 12345678908, 31415926590",
			"CPFs: <CPF>, 12345678908, <CPF>, 12345678908, <CPF>", ""},
		{"formatted values carry context", "CPF 123.456.789-09, 987.654.321-00, 12345678909", "CPF <CPF>, <CPF>, <CPF>", ""},
		{"at the edge of the window", "CPF 12345678909" + near + "98765432100", "CPF <CPF>" + near + "<CPF>", ""},
		{"beyond the window", "CPF 12345678909" + far + "98765432100", "CPF <CPF>" + far + "98765432100", "CPF <CPF>" + far + "<CPF>"},
		{"another entity does not carry context", "CPF 123.456.789-09 / " + "11223344556613", "CPF <CPF> / 11223344556613", "CPF <CPF> / <CNPJ>"},
		{"mixed list under one label", "CPF/CNPJ: 12345678909, 11223344556613, 98765432100, 31415926590", "CPF/CNPJ: <CPF>, <CNPJ>, <CPF>, <CPF>", ""},
		{"another entity does not end a list", "CPF: 12345678909, AB12CD34EF5602, 98765432100", "CPF: <CPF>, AB12CD34EF5602, <CPF>", "CPF: <CPF>, <CNPJ>, <CPF>"},
		{"formatted value without context", "123.456.789-09 pedido 98765432100", "<CPF> pedido 98765432100", "<CPF> pedido <CPF>"},
		{"keyword after, two values", "12345678909 e 98765432100 são meus CPFs", "<CPF> e <CPF> são meus CPFs", ""},
		{"keyword after, three values", "12345678909, 98765432100, 31415926590 são CPFs", "<CPF>, <CPF>, <CPF> são CPFs", ""},
		{"keyword after, a formatted value last", "12345678909 e 987.654.321-00 são CPFs", "<CPF> e <CPF> são CPFs", ""},
		{"keyword after, a formatted value in the middle", "12345678909, 987.654.321-00, 98765432100 são CPFs", "<CPF>, <CPF>, <CPF> são CPFs", ""},
		{"keyword after, the first value too far", "12345678909" + far + "98765432100 é meu CPF", "12345678909" + far + "<CPF> é meu CPF", "<CPF>" + far + "<CPF> é meu CPF"},
		{"keyword after, another entity in between", "12345678909 11223344556613 98765432100 é meu CPF", "<CPF> 11223344556613 <CPF> é meu CPF", "<CPF> <CNPJ> <CPF> é meu CPF"},
		{"a value that has context does not reach back", "11111111111 e CPF: 12345678909 e 98765432100 são CPFs", "11111111111 e CPF: <CPF> e <CPF> são CPFs", "<CPF> e CPF: <CPF> e <CPF> são CPFs"},
		{"a label does not reach back", "pedido 12345678909, CPF 98765432100", "pedido 12345678909, CPF <CPF>", "pedido <CPF>, CPF <CPF>"},
		{"a formatted value does not reach back", "pedido 12345678909, 987.654.321-00", "pedido 12345678909, <CPF>", "pedido <CPF>, <CPF>"},
		// The last two characters of the first candidate start the second one, a formatted CNPJ.
		{"overlapping candidates", "ab12cd34/ef56-02.ABC.DEF/0001-82 é o cnpj", "ab12cd34/ef56-<CNPJ> é o cnpj", "<CNPJ>.ABC.DEF/0001-82 é o cnpj"},
		// The same, with candidates of different entities and a later keyword that could reach back.
		{"overlapping candidates of two entities", "123456789-09.ABC.DEF/0001-21 e 98765432100 são CPFs", "123456789-<CNPJ> e <CPF> são CPFs", "<CPF>.ABC.DEF/0001-21 e <CPF> são CPFs"},
	})

	// Values pulled in by a later keyword stay in order among the others.
	got := mustNew(t).Find("12345678909, 987.654.321-00, 98765432100 são CPFs")
	want := []redactbr.Match{{Entity: redactbr.CPF, Start: 0, End: 11}, {Entity: redactbr.CPF, Start: 13, End: 27}, {Entity: redactbr.CPF, Start: 29, End: 40}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("Find = %v, want %v", got, want)
	}
}

func TestEscapeSequencesWrittenOut(t *testing.T) {
	runRedactCases(t, []redactCase{
		{"formatted CPF after a line break", `"lista:\n123.456.789-09\n987.654.321-00"`, `"lista:\n<CPF>\n<CPF>"`, ""},
		{"formatted CPF after a tab", `a\t123.456.789-09`, `a\t<CPF>`, ""},
		{"formatted CPF after a carriage return", `a\r\n123.456.789-09`, `a\r\n<CPF>`, ""},
		{"formatted CNPJ", `a\n11.223.344/5566-13`, `a\n<CNPJ>`, ""},
		{"CNPJ without dots", `a\nAB12CD34/EF56-02`, `a\n<CNPJ>`, ""},
		{"unformatted with a keyword", `CPF:\n12345678909`, `CPF:\n<CPF>`, ""},
		{"unformatted without a keyword", `a\n12345678909`, `a\n12345678909`, `a\n<CPF>`},
		{"list in a column", `cpf\n12345678909\n98765432100\n31415926590\n12345678909`, `cpf\n<CPF>\n<CPF>\n<CPF>\n<CPF>`, ""},
		{"email", `a\nana@example.com`, `a\n<EMAIL>`, ""},
		{"other escape letters are letters", `a\x123.456.789-09`, `a\x123.456.789-09`, ""},
		{"the letter without a backslash", `a n123.456.789-09`, `a n123.456.789-09`, ""},
		{"a backslash alone is a boundary", `C:\123.456.789-09`, `C:\<CPF>`, ""},
		{"a value starting with an escape letter", `C:\new12345678901`, `C:\new12345678901`, ""},
	})
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
