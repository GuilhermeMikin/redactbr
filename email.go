package redactbr

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Length limits from RFC 5321, section 4.5.3.1: a local part has at most 64 octets and a domain at most
// 255 octets. Expansion around '@' stops there, which also bounds the work per '@'.
const (
	maxLocalLen  = 64
	maxDomainLen = 255
)

// findEmails appends email matches to ms. It recognizes a pragmatic subset of the RFC 5322 addr-spec:
// a dot-atom local part made of ASCII letters, digits and "._%+-" (and letters or digits of other
// scripts), "@", and a domain of at least two dot-separated labels made of letters, digits and hyphens,
// whose last label has at least two letters (or is an "xn--" label). Quoted local parts, comments and IP
// address literals are not recognized.
func findEmails(s string, ms []Match) []Match {
	floor := 0 // a local part may not reach back into the previous address ("a@b.co@c.co")
	for i := 0; i < len(s); i++ {
		if s[i] != '@' {
			continue
		}
		start, ok := localStart(s, floor, i)
		if !ok {
			continue
		}
		end, ok := domainEnd(s, i+1)
		if !ok {
			continue
		}
		ms = append(ms, Match{Entity: Email, Start: start, End: end})
		floor = end
		i = end - 1
	}
	return ms
}

// localStart returns where the local part that ends at s[at] (the '@') begins, looking no further back
// than s[floor].
func localStart(s string, floor, at int) (int, bool) {
	j := at
	for j > floor {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		if !isLocalRune(r) {
			break
		}
		j -= size
		if at-j > maxLocalLen {
			return 0, false
		}
	}
	// Leading dots are punctuation ("see:...john@example.com"), not part of a dot-atom.
	for j < at && s[j] == '.' {
		j++
	}
	return j, j < at
}

// domainEnd returns where the domain that starts at s[from] ends.
func domainEnd(s string, from int) (int, bool) {
	j := from
	for j < len(s) {
		r, size := utf8.DecodeRuneInString(s[j:])
		if !isDomainRune(r) {
			break
		}
		j += size
		if j-from > maxDomainLen {
			return 0, false
		}
	}
	// Trailing dots and hyphens are punctuation ("write to john@example.com.").
	for j > from && (s[j-1] == '.' || s[j-1] == '-') {
		j--
	}
	return j, validDomain(s[from:j])
}

func validDomain(d string) bool {
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
	}
	tld := labels[len(labels)-1]
	if strings.HasPrefix(tld, "xn--") {
		return len(tld) > len("xn--")
	}
	n := 0
	for _, r := range tld {
		if !unicode.IsLetter(r) {
			return false
		}
		n++
	}
	return n >= 2
}

func isLocalRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r) || strings.ContainsRune("._%+-", r)
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isDomainRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r) || r == '.' || r == '-'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
