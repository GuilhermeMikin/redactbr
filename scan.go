package redactbr

import (
	"cmp"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/GuilhermeMikin/redactbr/internal/cnpj"
	"github.com/GuilhermeMikin/redactbr/internal/cpf"
)

// contextWindow is how many bytes before an unformatted candidate are searched for its keyword under the
// Balanced policy.
const contextWindow = 32

const (
	cpfKeyword  = "cpf"
	cnpjKeyword = "cnpj"
)

// find returns the matches in s, sorted by offset and without overlaps. It runs in time linear in len(s):
// identifier candidates start only at token boundaries and are at most 18 bytes long, and email expansion
// around each '@' is bounded by the RFC 5321 length limits.
func (r *Redactor) find(s string) []Match {
	var ms []Match
	if r.cpf || r.cnpj {
		ms = r.findIdentifiers(s, ms)
	}
	if r.email {
		n := len(ms)
		ms = findEmails(s, ms)
		if n > 0 && len(ms) > n {
			ms = resolveOverlaps(ms)
		}
	}
	return ms
}

// findIdentifiers appends CPF and CNPJ matches to ms.
func (r *Redactor) findIdentifiers(s string, ms []Match) []Match {
	for i := 0; i < len(s); {
		if !isASCIIAlnum(s[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isASCIIAlnum(s[j]) {
			j++
		}
		if mayStartIdentifier(s, i, j) && leftBoundary(s, i) {
			if m, ok := r.identifierAt(s, i); ok {
				ms = append(ms, m)
				i = m.End
				continue
			}
		}
		// No candidate can start inside a run of letters and digits: its left neighbor would not be a
		// boundary.
		i = j
	}
	return ms
}

// mayStartIdentifier is a cheap filter on the run of ASCII letters and digits s[i:j]: every recognized
// shape starts with a run of exactly 2 or 3 characters followed by '.', 8 followed by '/', 9 followed by
// '-', or is a run of exactly 11 or 14 characters.
func mayStartIdentifier(s string, i, j int) bool {
	var sep byte
	if j < len(s) {
		sep = s[j]
	}
	switch j - i {
	case 11, 14:
		return true
	case 2, 3:
		return sep == '.'
	case 8:
		return sep == '/'
	case 9:
		return sep == '-'
	}
	return false
}

// identifierAt reports the CNPJ or CPF that starts at s[i], if any, applying shape, boundary, check-digit
// and policy rules in that order.
func (r *Redactor) identifierAt(s string, i int) (Match, bool) {
	if r.cnpj {
		if end, c, formatted, ok := scanCNPJ(s, i); ok && rightBoundary(s, end) && cnpj.Valid(c) &&
			r.accept(s, i, formatted, cnpjKeyword) {
			return Match{Entity: CNPJ, Start: i, End: end}, true
		}
	}
	if r.cpf {
		if end, d, formatted, ok := scanCPF(s, i); ok && rightBoundary(s, end) && cpf.Valid(d) &&
			r.accept(s, i, formatted, cpfKeyword) {
			return Match{Entity: CPF, Start: i, End: end}, true
		}
	}
	return Match{}, false
}

// accept applies the detection policy to a candidate whose check digits are valid.
func (r *Redactor) accept(s string, start int, formatted bool, keyword string) bool {
	return formatted || r.policy == Strict || hasKeyword(s, start, keyword)
}

// scanCPF matches a CPF shape at s[i]: DDD.DDD.DDD-DD (formatted), DDDDDDDDD-DD or DDDDDDDDDDD
// (unformatted). It returns the end offset and the eleven digits.
func scanCPF(s string, i int) (end int, d [cpf.Len]byte, formatted, ok bool) {
	if i+14 <= len(s) && s[i+3] == '.' && s[i+7] == '.' && s[i+11] == '-' &&
		digitsInto(d[0:3], s[i:i+3]) && digitsInto(d[3:6], s[i+4:i+7]) &&
		digitsInto(d[6:9], s[i+8:i+11]) && digitsInto(d[9:11], s[i+12:i+14]) {
		return i + 14, d, true, true
	}
	if i+12 <= len(s) && s[i+9] == '-' && digitsInto(d[0:9], s[i:i+9]) && digitsInto(d[9:11], s[i+10:i+12]) {
		return i + 12, d, false, true
	}
	if i+11 <= len(s) && digitsInto(d[:], s[i:i+11]) {
		return i + 11, d, false, true
	}
	return 0, d, false, false
}

// scanCNPJ matches a CNPJ shape at s[i]: XX.XXX.XXX/XXXX-DD or XXXXXXXX/XXXX-DD (formatted), or
// XXXXXXXXXXXXDD (unformatted), where X is an ASCII letter or digit and D an ASCII digit. It returns the
// end offset and the fourteen characters, with letters folded to uppercase.
func scanCNPJ(s string, i int) (end int, c [cnpj.Len]byte, formatted, ok bool) {
	if i+18 <= len(s) && s[i+2] == '.' && s[i+6] == '.' && s[i+10] == '/' && s[i+15] == '-' &&
		alnumInto(c[0:2], s[i:i+2]) && alnumInto(c[2:5], s[i+3:i+6]) && alnumInto(c[5:8], s[i+7:i+10]) &&
		alnumInto(c[8:12], s[i+11:i+15]) && digitsInto(c[12:14], s[i+16:i+18]) {
		return i + 18, c, true, true
	}
	if i+16 <= len(s) && s[i+8] == '/' && s[i+13] == '-' &&
		alnumInto(c[0:8], s[i:i+8]) && alnumInto(c[8:12], s[i+9:i+13]) && digitsInto(c[12:14], s[i+14:i+16]) {
		return i + 16, c, true, true
	}
	if i+14 <= len(s) && alnumInto(c[0:12], s[i:i+12]) && digitsInto(c[12:14], s[i+12:i+14]) {
		return i + 14, c, false, true
	}
	return 0, c, false, false
}

// digitsInto copies src into dst if src is all ASCII digits.
func digitsInto(dst []byte, src string) bool {
	for j := range len(src) {
		b := src[j]
		if b < '0' || b > '9' {
			return false
		}
		dst[j] = b
	}
	return true
}

// alnumInto copies src into dst, folding ASCII letters to uppercase, if src is all ASCII letters and
// digits.
func alnumInto(dst []byte, src string) bool {
	for j := range len(src) {
		b := src[j]
		switch {
		case '0' <= b && b <= '9', 'A' <= b && b <= 'Z':
		case 'a' <= b && b <= 'z':
			b -= 'a' - 'A'
		default:
			return false
		}
		dst[j] = b
	}
	return true
}

// leftBoundary reports whether a candidate may start at s[i]: the previous rune must not be a letter or
// digit (in any script), and must not be a '.' that follows an ASCII digit, which would make the
// candidate the tail of a decimal or dotted number.
func leftBoundary(s string, i int) bool {
	if i == 0 {
		return true
	}
	r, size := utf8.DecodeLastRuneInString(s[:i])
	if isWordRune(r) {
		return false
	}
	decimalTail := r == '.' && i-size > 0 && isASCIIDigit(s[i-size-1])
	return !decimalTail
}

// rightBoundary reports whether a candidate may end at s[end]: the next rune must not be a letter or digit,
// and must not be a '.' followed by an ASCII digit.
func rightBoundary(s string, end int) bool {
	if end == len(s) {
		return true
	}
	r, size := utf8.DecodeRuneInString(s[end:])
	if isWordRune(r) {
		return false
	}
	decimalHead := r == '.' && end+size < len(s) && isASCIIDigit(s[end+size])
	return !decimalHead
}

// hasKeyword reports whether keyword (lowercase ASCII) occurs, in any case, in the contextWindow bytes
// before start.
func hasKeyword(s string, start int, keyword string) bool {
	window := s[max(0, start-contextWindow):start]
	for j := 0; j+len(keyword) <= len(window); j++ {
		k := 0
		for k < len(keyword) && lowerASCII(window[j+k]) == keyword[k] {
			k++
		}
		if k == len(keyword) {
			return true
		}
	}
	return false
}

// resolveOverlaps sorts ms by offset and drops every match that overlaps an earlier one. Between matches
// that start at the same offset, the longest wins.
func resolveOverlaps(ms []Match) []Match {
	slices.SortFunc(ms, func(a, b Match) int {
		if a.Start != b.Start {
			return cmp.Compare(a.Start, b.Start)
		}
		return cmp.Compare(b.End, a.End)
	})
	out := ms[:0]
	end := 0
	for _, m := range ms {
		if m.Start < end {
			continue
		}
		out = append(out, m)
		end = m.End
	}
	return out
}

func isWordRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r)
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isASCIIAlnumRune(r rune) bool {
	return ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

func isASCIIAlnum(b byte) bool {
	return isASCIIDigit(b) || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z')
}

func isASCIIDigit(b byte) bool {
	return '0' <= b && b <= '9'
}

func lowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
