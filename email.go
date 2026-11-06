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

// localSeparators are the punctuation allowed in a local part.
const localSeparators = "._%+-'"

// findEmails appends email matches to ms. It follows the RFC 5322 addr-spec pragmatically, not strictly:
// a local part made of ASCII letters, digits and "._%+-'" (and letters, digits and combining
// marks of other scripts), "@", and a domain of at least two dot-separated labels made of letters, digits
// and hyphens, whose last label passes validTLD. Quoted local parts, comments and IP address literals are not
// recognized.
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
	if j < at && followsEscape(s, j) {
		j++ // the letter of a written-out line break or tab is not part of the address
	}
	// Text glued before an address can make the run too long ("relatorio-mensal-…-ana@example.com"). The address
	// then starts after a separator, as far back as the limit allows.
	if at-j > maxLocalLen {
		k := strings.IndexAny(s[at-maxLocalLen-1:at], localSeparators)
		if k < 0 {
			return j, false
		}
		j = at - maxLocalLen + k
	}
	// Leading dots and apostrophes are punctuation ("see:...ana@example.com", "'ana@example.com'").
	for j < at && (s[j] == '.' || s[j] == '\'') {
		j++
	}
	return j, j < at
}

// domainEnd returns where the domain that starts at s[from] ends. Text glued to an address
// ("ana@example.com-thanks", "ana@example.com.2", trailing dots) is not part of it: the longest prefix that ends
// before a '.' or '-' and forms a valid domain wins. A valid domain has at least two labels, none of them empty
// or starting or ending with '-', and a last label accepted by validTLD. At most maxDomainLen+1 bytes are scanned,
// once: the labels before the last one are checked as the scan passes them.
func domainEnd(s string, from int) (int, bool) {
	j := from
	for j < len(s) && j-from <= maxDomainLen {
		r, size := utf8.DecodeRuneInString(s[j:])
		if !isDomainRune(r) {
			break
		}
		j += size
	}
	best, labels, start := -1, 0, from
	hyphen := false // the current label has a '-' so far, which only an "xn--" label may have as a last label
	for i := from; i <= j; i++ {
		if i < j && s[i] != '.' && s[i] != '-' {
			continue
		}
		label := s[start:i]
		valid := label != "" && label[0] != '-' && label[len(label)-1] != '-'
		if valid && labels > 0 && i-from <= maxDomainLen && (!hyphen || hasXNPrefix(label)) && validTLD(label) {
			best = i
		}
		if i == j || s[i] == '-' {
			hyphen = true
			continue
		}
		if !valid {
			break // a whole label is invalid, so no longer prefix can be valid
		}
		labels++
		start, hyphen = i+1, false
	}
	return best, best >= 0
}

func hasXNPrefix(label string) bool {
	return len(label) > len("xn--") && strings.EqualFold(label[:len("xn--")], "xn--")
}

// validTLD reports whether the last label is an internationalized "xn--" label, or has at least two letters
// (with any combining marks) and then, optionally, digits, so that text glued to it ("example.com2") or a label in
// another script does not hide an address. Digits alone or before letters are not valid: that rules out IP addresses,
// units ("2.6GHz") and versions ("1.2.3rc1").
func validTLD(tld string) bool {
	if hasXNPrefix(tld) {
		return true
	}
	letters, digits := 0, false
	for _, r := range tld {
		switch {
		case unicode.IsLetter(r) || unicode.IsMark(r):
			if digits {
				return false
			}
			if unicode.IsLetter(r) {
				letters++
			}
		case unicode.IsDigit(r):
			digits = true
		default:
			return false
		}
	}
	return letters >= 2
}

func isLocalRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r) || strings.ContainsRune(localSeparators, r)
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

func isDomainRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r) || r == '.' || r == '-'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}
