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
	redactbr.CPF:  regexp.MustCompile(`(?i)c\.*p\.*f`),
	redactbr.CNPJ: regexp.MustCompile(`(?i)c\.*n\.*p\.*j`),
}

var (
	oracleGapStop = regexp.MustCompile(`[0-9\n\r.;!?]|\\[nr]`)
	oracleLabel   = regexp.MustCompile(`^[ \t"'\\\-(\[#]*[:=0-9]`)
)

func oracleIsWord(r rune) bool {
	return r != 'ª' && r != 'º' && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r))
}

// oracleKeywordAfter restates "Keyword after" in spec §7.
func oracleKeywordAfter(s string, end int, entity redactbr.Entity) bool {
	window := s[end:min(len(s), end+32)]
	for _, m := range oracleKeywords[entity].FindAllStringIndex(window, -1) {
		if oracleGapStop.MatchString(window[:m[0]]) {
			return false
		}
		from, to := end+m[0], end+m[1]
		if to < len(s) && (s[to] == 's' || s[to] == 'S') {
			to++
		}
		left, n := utf8.DecodeLastRuneInString(s[:from])
		right, _ := utf8.DecodeRuneInString(s[to:])
		escape := (left == 'n' || left == 'r' || left == 't') && from-n > 0 && s[from-n-1] == '\\'
		if left == '_' || right == '_' || (oracleIsWord(left) && !escape) || oracleIsWord(right) {
			continue
		}
		if !oracleLabel.MatchString(s[to:min(len(s), to+32)]) {
			return true
		}
	}
	return false
}

type oracleValue struct {
	redactbr.Match
	context, accepted bool
	overlapped        bool // a later value of the other entity starts inside this one
}

// oracleValid reports whether m, in one of the shapes of entity, has valid check digits.
func oracleValid(entity redactbr.Entity, m string) bool {
	v := strings.ToUpper(strings.NewReplacer(".", "", "-", "", "/", "").Replace(m))
	if entity == redactbr.CNPJ {
		var c [cnpj.Len]byte
		copy(c[:], v)
		return cnpj.Valid(c)
	}
	var d [cpf.Len]byte
	copy(d[:], v)
	return cpf.Valid(d)
}

// oracleFormattedAt reports whether a formatted value starts at s[i].
func oracleFormattedAt(s string, i int) bool {
	for _, sh := range oracleShapes {
		if m := sh.re.FindString(s[i:]); sh.formatted && m != "" && oracleValid(sh.entity, m) {
			if r, _ := utf8.DecodeRuneInString(s[i+len(m):]); !oracleIsWord(r) {
				return true
			}
		}
	}
	return false
}

func oracleFind(s string, strict bool) []redactbr.Match {
	isDigit := func(b byte) bool { return '0' <= b && b <= '9' }
	var valid []oracleValue // every valid candidate so far, whether it was accepted or not
	lastAccepted := -1      // where the last accepted value ends
	end := 0
	for i := 0; i < len(s); i++ {
		if i < end {
			continue // inside an accepted match
		}
		for _, sh := range oracleShapes {
			m := sh.re.FindString(s[i:])
			if m == "" {
				continue
			}
			stop := i + len(m)
			if i > 0 {
				r, n := utf8.DecodeLastRuneInString(s[:i])
				escape := (r == 'n' || r == 'r' || r == 't') && i-n > 0 && s[i-n-1] == '\\'
				if oracleIsWord(r) && !escape {
					continue
				}
				if !sh.formatted && r == '.' && i-n > 0 && isDigit(s[i-n-1]) && lastAccepted != i-1 {
					continue
				}
			}
			if stop < len(s) {
				r, n := utf8.DecodeRuneInString(s[stop:])
				if oracleIsWord(r) {
					continue
				}
				if !sh.formatted && r == '.' && stop+n < len(s) && isDigit(s[stop+n]) && !oracleFormattedAt(s, stop+n) {
					continue
				}
			}
			if !oracleValid(sh.entity, m) {
				continue
			}

			cur := oracleValue{Match: redactbr.Match{Entity: sh.entity, Start: i, End: stop}}
			// previous returns the index of the last value of this entity before k, or -1.
			previous := func(k int) int {
				for k--; k >= 0; k-- {
					if valid[k].Entity == sh.entity {
						return k
					}
				}
				return -1
			}
			inRun := func(prev, next oracleValue) bool { return next.Start >= prev.End && next.Start-prev.End <= 32 }
			for k := range valid {
				if valid[k].Entity != sh.entity && valid[k].End > i {
					valid[k].overlapped = true
				}
			}
			last := previous(len(valid))
			before := oracleKeywords[sh.entity].MatchString(s[max(0, i-32):i]) ||
				(last >= 0 && valid[last].context && inRun(valid[last], cur))
			after := !before && oracleKeywordAfter(s, stop, sh.entity)
			cur.context = before || after
			if after {
				// The keyword also reaches back over the values of this run.
				next := cur
				for k := last; k >= 0 && inRun(valid[k], next); k = previous(k) {
					valid[k].accepted = valid[k].accepted || !valid[k].overlapped
					next = valid[k]
				}
			}
			cur.accepted = sh.formatted || strict || cur.context
			valid = append(valid, cur)
			if cur.accepted {
				end = stop
				lastAccepted = stop
				break // shapes are listed longest first; a shorter one at the same offset cannot win
			}
		}
	}
	var out []redactbr.Match
	for _, v := range valid {
		if v.accepted {
			out = append(out, v.Match)
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
		`\"`, `\r`,
		"\n", "\r", `\n`, `\t`, `\x`, "\\", ";", "!", "?", ":", "=", "\"", "'", "(", ")", "[", "#", "\t",
		" cpf", " CNPJ", " CPFs", " cnpjs", " (cpf)", " é meu CPF", " é o cnpj", "_cpf", "cpf_", " de ",
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
		"12345678909 é meu CPF", "id=12345678909 cpf=98765432100", `{"id":"12345678909","cpf":"x"}`,
		"12345678909, 987.654.321-00, 98765432100 são CPFs", "CPFs: 12345678909, 98765432100, 31415926590, 12345678909",
		"CPF/CNPJ: 12345678909, 11223344556613, 98765432100, 31415926590", "123456789-09.ABC.DEF/0001-21 e 98765432100 são CPFs",
		`a\n123.456.789-09`, `CPF:\t12345678909`, "ab12cd34/ef56-02.ABC.DEF/0001-82 é o cnpj",
		`{\"doc\":\"12345678909\",\"tipo\":\"CPF\"}`, `{\"id\":\"12345678909\",\"cpf\":\"x\"}`, `12345678909\tCPF`,
		"1.123.456.789-09", "123.456.789-09.1",
	} {
		f.Add(s)
	}
	balanced := mustNew(f, redactbr.WithEntities(redactbr.CPF, redactbr.CNPJ))
	strict := mustNew(f, redactbr.WithEntities(redactbr.CPF, redactbr.CNPJ), redactbr.WithPolicy(redactbr.Strict))
	f.Fuzz(func(t *testing.T, s string) {
		checkAgainstOracle(t, balanced, strict, s)
	})
}

// Under Strict nothing detectable may be left in the output, so replacing one value must not make another one
// detectable. Text dense in values next to separators finds that faster than fuzzing does.
func TestStrictLeavesNothingDetectable(t *testing.T) {
	strict := mustNew(t, redactbr.WithPolicy(redactbr.Strict))
	rng := rand.New(rand.NewPCG(31, 32))
	separators := []string{".", ".", ".", "-", "/", "_", " ", ",", "'", "+", "@", "@b.co", "@b.co.", "\\n", ""}
	values := func() string {
		switch rng.IntN(6) {
		case 0:
			return generatedCPF(rng, rng.IntN(3))
		case 1:
			return generatedCNPJ(rng, rng.IntN(3))
		case 2:
			return strings.Repeat("0", rng.IntN(70))
		case 3:
			return "a@b.co"
		case 4:
			return []string{"12345678909", "123.456.789-09", "11223344556613", "AB12CD34/EF56-02", "1", "12"}[rng.IntN(6)]
		}
		return strings.Repeat("a.", rng.IntN(40))
	}
	for range 100000 {
		var b strings.Builder
		for range 1 + rng.IntN(5) {
			b.WriteString(values())
			b.WriteString(separators[rng.IntN(len(separators))])
		}
		s := b.String()
		if left := strict.Find(strict.Redact(s)); len(left) > 0 {
			t.Fatalf("Redact(%q) = %q, which still has %v", s, strict.Redact(s), left)
		}
	}
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
