package redactbr

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Length limits from RFC 5321, section 4.5.3.1: a local part has at most 64 octets and a domain at most
// 255 octets.
const (
	maxLocalLen  = 64
	maxDomainLen = 255
)

// findEmails appends email matches to ms. It recognizes a pragmatic subset of the RFC 5322 addr-spec:
// a dot-atom local part made of ASCII letters, digits and "._%+-'" (and letters, digits and combining
// marks of other scripts), "@", and a domain of at least two dot-separated labels made of letters, digits
// and hyphens, whose last label has at least two letters (or is an "xn--" label). Quoted local parts,
// comments and IP address literals are not recognized.
func findEmails(s string, ms []Match) []Match {
	floor := 0 // a local part may not reach back into the previous address ("a@b.co@c.co")
	for i := 0; i < len(s); i++ {
		j := strings.IndexByte(s[i:], '@')
		if j < 0 {
			break
		}
		i += j
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
// than s[floor]. The walk stops at the first rune that cannot be part of a local part, so across all '@'
// in a text every byte is visited at most once.
func localStart(s string, floor, at int) (int, bool) {
	j := at
	for j > floor {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		if !isLocalRune(r) {
			break
		}
		j -= size
	}
	// Leading dots and apostrophes are punctuation ("see:...ana@example.com", "'ana@example.com'").
	for j < at && (s[j] == '.' || s[j] == '\'') {
		j++
	}
	return j, j < at && at-j <= maxLocalLen
}

// domainEnd returns where the domain that starts at s[from] ends. Text glued to an address
// ("ana@example.com-thanks", "ana@example.com.2", trailing dots) is not part of it: the longest prefix
// that ends before a '.' or '-' and forms a valid domain wins. At most maxDomainLen+1 bytes are scanned.
func domainEnd(s string, from int) (int, bool) {
	j := from
	for j < len(s) && j-from <= maxDomainLen {
		r, size := utf8.DecodeRuneInString(s[j:])
		if !isDomainRune(r) {
			break
		}
		j += size
	}
	for end := j; end > from; {
		if end-from <= maxDomainLen && validDomain(s[from:end]) {
			return end, true
		}
		k := strings.LastIndexAny(s[from:end], ".-")
		if k < 0 {
			break
		}
		end = from + k
	}
	return 0, false
}

func validDomain(d string) bool {
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

// validTLD reports whether the last label is at least two letters, or an internationalized "xn--" label.
func validTLD(tld string) bool {
	if len(tld) > len("xn--") && strings.EqualFold(tld[:len("xn--")], "xn--") {
		return true
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
		return isASCIIAlnumRune(r) || strings.ContainsRune("._%+-'", r)
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

func isDomainRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r) || r == '.' || r == '-'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}
