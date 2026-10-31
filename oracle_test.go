package redactbr_test

import (
	"math/rand/v2"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/GuilhermeMikin/redactbr"
	"github.com/GuilhermeMikin/redactbr/internal/cnpj"
	"github.com/GuilhermeMikin/redactbr/internal/cpf"
)

// The oracle restates the identifier detection rules of docs/spec/v0.1.md as directly as possible:
// regular expressions for the shapes, every byte offset as a possible start, greedy left-to-right
// acceptance.
// It is slow on purpose; agreement with Find is evidence that the scanner's shortcuts (skipping runs,
// trying shapes in a fixed order) do not change the result.

var oracleShapes = []struct {
	entity    redactbr.Entity
	re        *regexp.Regexp
	formatted bool
}{
	{redactbr.CNPJ, regexp.MustCompile(`^[0-9A-Za-z]{2}\.[0-9A-Za-z]{3}\.[0-9A-Za-z]{3}/[0-9A-Za-z]{4}-[0-9]{2}`), true},
	{redactbr.CNPJ, regexp.MustCompile(`^[0-9A-Z]{8}/[0-9A-Z]{4}-[0-9]{2}`), true},
	{redactbr.CNPJ, regexp.MustCompile(`^[0-9A-Za-z]{8}/[0-9A-Za-z]{4}-[0-9]{2}`), false}, // with lowercase
	{redactbr.CNPJ, regexp.MustCompile(`^[0-9A-Za-z]{12}[0-9]{2}`), false},
	{redactbr.CPF, regexp.MustCompile(`^[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2}`), true},
	{redactbr.CPF, regexp.MustCompile(`^[0-9]{9}-[0-9]{2}`), false},
	{redactbr.CPF, regexp.MustCompile(`^[0-9]{11}`), false},
}

var oracleKeywords = map[redactbr.Entity]*regexp.Regexp{
	redactbr.CPF:  regexp.MustCompile(`c\.*p\.*f`),
	redactbr.CNPJ: regexp.MustCompile(`c\.*n\.*p\.*j`),
}

func oracleFind(s string, strict bool) []redactbr.Match {
	isWord := func(r rune) bool {
		return r != 'ª' && r != 'º' && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r))
	}
	isDigit := func(b byte) bool { return '0' <= b && b <= '9' }
	var out []redactbr.Match
	end := 0
	for i := 0; i < len(s); i++ {
		if i < end {
			continue // inside an accepted match
		}
		afterMatch := len(out) > 0 && out[len(out)-1].End == i-1
		for _, sh := range oracleShapes {
			m := sh.re.FindString(s[i:])
			if m == "" {
				continue
			}
			stop := i + len(m)
			if i > 0 {
				r, n := utf8.DecodeLastRuneInString(s[:i])
				if isWord(r) || (r == '.' && i-n > 0 && isDigit(s[i-n-1]) && !afterMatch) {
					continue
				}
			}
			if stop < len(s) {
				r, n := utf8.DecodeRuneInString(s[stop:])
				if isWord(r) || (r == '.' && stop+n < len(s) && isDigit(s[stop+n])) {
					continue
				}
			}
			v := strings.ToUpper(strings.NewReplacer(".", "", "-", "", "/", "").Replace(m))
			if sh.entity == redactbr.CNPJ {
				var c [cnpj.Len]byte
				copy(c[:], v)
				if !cnpj.Valid(c) {
					continue
				}
			} else {
				var d [cpf.Len]byte
				copy(d[:], v)
				if !cpf.Valid(d) {
					continue
				}
			}
			window := strings.ToLower(s[max(0, i-32):i])
			if sh.formatted || strict || oracleKeywords[sh.entity].MatchString(window) {
				out = append(out, redactbr.Match{Entity: sh.entity, Start: i, End: stop})
				end = stop
				break // shapes are listed longest first; a shorter one at the same offset cannot win
			}
		}
	}
	return out
}

// randomText produces short strings dense in near-misses: digits, letters, separators, keywords,
// non-ASCII letters and generated identifiers with valid or broken check digits.
func randomText(rng *rand.Rand) string {
	pieces := []string{
		"0", "1", "5", "9", "a", "X", "z", ".", "-", "/", " ", ",", "_", "é", "\u00a0", "\xff",
		"cpf", "CNPJ", "Cpf:", "cnpj=", "C.P.F.", "c.n.p.j", "nº", "ª", "\u0301", "/", "123", "456.", "0001", "-09",
	}
	var b strings.Builder
	for range rng.IntN(24) {
		switch rng.IntN(5) {
		case 0:
			b.WriteString(generatedCPF(rng, rng.IntN(3)))
		case 1:
			b.WriteString(generatedCNPJ(rng, rng.IntN(3)))
		default:
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
	}
	return b.String()
}

// generatedCPF returns a CPF built from random digits, in one of the three recognized shapes; one in
// four has a corrupted check digit.
func generatedCPF(rng *rand.Rand, shape int) string {
	var base [cpf.BaseLen]byte
	for i := range base {
		base[i] = byte('0' + rng.IntN(10))
	}
	dv1, dv2, _ := cpf.CheckDigits(base)
	if rng.IntN(4) == 0 {
		dv2 = '0' + (dv2-'0'+1)%10
	}
	d := string(base[:]) + string([]byte{dv1, dv2})
	switch shape {
	case 0:
		return d[0:3] + "." + d[3:6] + "." + d[6:9] + "-" + d[9:]
	case 1:
		return d[0:9] + "-" + d[9:]
	default:
		return d
	}
}

// generatedCNPJ returns a CNPJ built from random characters, in one of the three recognized shapes,
// sometimes lowercase; one in four has a corrupted check digit.
func generatedCNPJ(rng *rand.Rand, shape int) string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var base [cnpj.BaseLen]byte
	for i := range base {
		base[i] = alphabet[rng.IntN(len(alphabet))]
	}
	dv1, dv2, _ := cnpj.CheckDigits(base)
	if rng.IntN(4) == 0 {
		dv1 = '0' + (dv1-'0'+1)%10
	}
	c := string(base[:]) + string([]byte{dv1, dv2})
	if rng.IntN(4) == 0 {
		c = strings.ToLower(c)
	}
	switch shape {
	case 0:
		return c[0:2] + "." + c[2:5] + "." + c[5:8] + "/" + c[8:12] + "-" + c[12:]
	case 1:
		return c[0:8] + "/" + c[8:12] + "-" + c[12:]
	default:
		return c
	}
}

func checkAgainstOracle(t *testing.T, balanced, strict *redactbr.Redactor, s string) {
	t.Helper()
	for _, p := range []struct {
		r      *redactbr.Redactor
		strict bool
	}{{balanced, false}, {strict, true}} {
		got, want := p.r.Find(s), oracleFind(s, p.strict)
		if len(got) != len(want) {
			t.Fatalf("strict=%v: Find returned %v, oracle %v (input of %d bytes)", p.strict, got, want, len(s))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("strict=%v: Find returned %v, oracle %v (input of %d bytes)", p.strict, got, want, len(s))
			}
		}
	}
}

func TestFindMatchesOracle(t *testing.T) {
	balanced := mustNew(t, redactbr.WithEntities(redactbr.CPF, redactbr.CNPJ))
	strict := mustNew(t, redactbr.WithEntities(redactbr.CPF, redactbr.CNPJ), redactbr.WithPolicy(redactbr.Strict))
	rng := rand.New(rand.NewPCG(11, 12))
	for range 10000 {
		checkAgainstOracle(t, balanced, strict, randomText(rng))
	}
}

func FuzzFindMatchesOracle(f *testing.F) {
	for _, s := range []string{
		"CPF 123.456.789-09", "cpf 12345678909", "1.123.456.789-09", "12345678909.5", "cnpj ab12cd34ef5602",
		"AB12CD34/EF56-02", "x12345678909", "cpf_12345678909", "11.223.344/5566-13,987.654.321-00",
	} {
		f.Add(s)
	}
	balanced := mustNew(f, redactbr.WithEntities(redactbr.CPF, redactbr.CNPJ))
	strict := mustNew(f, redactbr.WithEntities(redactbr.CPF, redactbr.CNPJ), redactbr.WithPolicy(redactbr.Strict))
	f.Fuzz(func(t *testing.T, s string) {
		checkAgainstOracle(t, balanced, strict, s)
	})
}

// FuzzRedact checks invariants of Redact with every entity enabled: it never panics, its output is the
// input with exactly the Find matches replaced, each replacement grows the text by at most one byte, and
// under Strict nothing detectable is left in the output.
func FuzzRedact(f *testing.F) {
	for _, s := range []string{
		"CPF 123.456.789-09 / ana@example.com", "12345678909@example.com", "a@b.co", "@@@", "<CPF>",
		"cnpj AB.12C.D34/EF56-02.", strings.Repeat("a", 70) + "@example.com",
	} {
		f.Add(s)
	}
	balanced := mustNew(f)
	strict := mustNew(f, redactbr.WithPolicy(redactbr.Strict))
	f.Fuzz(func(t *testing.T, s string) {
		for _, r := range []*redactbr.Redactor{balanced, strict} {
			ms := r.Find(s)
			out := r.Redact(s)
			var want strings.Builder
			last := 0
			for _, m := range ms {
				if m.Start < last || m.End <= m.Start || m.End > len(s) {
					t.Fatalf("invalid or overlapping match %v", m)
				}
				want.WriteString(s[last:m.Start] + "<" + string(m.Entity) + ">")
				last = m.End
			}
			want.WriteString(s[last:])
			if out != want.String() {
				t.Fatal("Redact output differs from the input with the Find matches replaced")
			}
			if len(out) > len(s)+len(ms) {
				t.Fatalf("output grew by %d bytes for %d matches", len(out)-len(s), len(ms))
			}
		}
		if left := strict.Find(strict.Redact(s)); len(left) > 0 {
			t.Fatalf("Strict output still contains detectable values: %v", left)
		}
	})
}
