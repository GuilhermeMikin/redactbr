package redactbr

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

// referenceDomainEnd states the rule of domainEnd directly: from the longest candidate down, return the first
// that is a valid domain. It is quadratic in the length of the domain, which is why domainEnd does not do this.
func referenceDomainEnd(s string, from int) (int, bool) {
	j := from
	for j < len(s) && j-from <= maxDomainLen {
		r, size := utf8.DecodeRuneInString(s[j:])
		if !isDomainRune(r) {
			break
		}
		j += size
	}
	for end := j; end > from; {
		if end-from <= maxDomainLen && referenceValidDomain(s[from:end]) {
			return end, true
		}
		k := strings.LastIndexAny(s[from:end], ".-")
		if k < 0 {
			break
		}
		end = from + k
	}
	return -1, false
}

func referenceValidDomain(d string) bool {
	labels := 0
	for rest := d; ; {
		label, tail, more := strings.Cut(rest, ".")
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		labels++
		if !more {
			return labels >= 2 && validTLD(label)
		}
		rest = tail
	}
}

func checkDomainEnd(t *testing.T, s string) {
	t.Helper()
	got, gotOK := domainEnd(s, 0)
	want, wantOK := referenceDomainEnd(s, 0)
	if gotOK != wantOK || (gotOK && got != want) {
		t.Fatalf("domainEnd(%q) = (%d, %v), reference (%d, %v)", s, got, gotOK, want, wantOK)
	}
}

func TestDomainEndMatchesReference(t *testing.T) {
	for _, s := range []string{
		"", ".", "-", "a", "a.", ".a", "a.b", "a.bc", "a.bc.", "a.bc-", "a.bc-d", "a.bc-d.ef", "a-.bc", "-a.bc", "a..bc", "a.b.cd",
		"a.xn--p1ai", "a.XN--P1AI", "a.xn--", "a.xn---", "a.xn--p-", "a.xn--p-b", "a.bc.1", "a.bc.1d", "a.b-c", "a.bc-de-fg", "é.çã", "a.é",
		"example.com-thanks", "example.com.2", "example.com@x", strings.Repeat("a.", 127) + "bc", strings.Repeat("a.", 128) + "bc",
		strings.Repeat("a", 254) + ".b", strings.Repeat("a", 252) + ".bc", strings.Repeat("a", 253) + ".bc", strings.Repeat("a", 300) + ".bc",
		"a." + strings.Repeat("b", 253), "a." + strings.Repeat("b", 254), "ab.cd." + strings.Repeat("-", 300),
	} {
		checkDomainEnd(t, s)
	}
	pieces := []string{"a", "b", "Z", "1", ".", ".", "-", "-", "xn--", "XN--", "com", "é", "́", "@", " ", "\xff", strings.Repeat("a", 60), strings.Repeat("a.", 30), strings.Repeat("-a", 30)}
	rng := rand.New(rand.NewPCG(21, 22))
	for range 200000 {
		var b strings.Builder
		for range rng.IntN(12) {
			b.WriteString(pieces[rng.IntN(len(pieces))])
		}
		checkDomainEnd(t, b.String())
	}
}

func FuzzDomainEnd(f *testing.F) {
	for _, s := range []string{"example.com", "a.bc-d.ef", "a.xn--p-b", "é.çã", strings.Repeat("a.", 127) + "bc"} {
		f.Add(s)
	}
	f.Fuzz(checkDomainEnd)
}
