package redactbr

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GuilhermeMikin/redactbr/internal/cnpj"
	"github.com/GuilhermeMikin/redactbr/internal/cpf"
)

// contextWindow is how many bytes around an unformatted candidate are searched for its keyword under the
// Balanced policy, and how far apart two values of a list can be.
const contextWindow = 32

const (
	cpfKeyword  = "cpf"
	cnpjKeyword = "cnpj"
)

// find returns the matches in s, sorted by offset and without overlaps. Scanning is linear in len(s):
// identifier candidates start only at token boundaries and are at most 18 bytes long, the domain scan after
// each '@' is bounded, and walks back over local parts never revisit bytes. Matches are sorted when needed.
func (r *Redactor) find(s string) []Match {
	var emails, ids []Match
	if !r.noEmail {
		emails = findEmails(s, nil)
	}
	if !r.noCPF || !r.noCNPJ {
		ids = r.findIdentifiers(s, emails)
	}
	switch {
	case len(emails) == 0:
		return ids
	case len(ids) == 0:
		return emails
	}
	return resolveOverlaps(s, append(ids, emails...))
}

// findIdentifiers returns the CPF and CNPJ matches in s, sorted. emails are the email matches, sorted; a
// '.' right after one of them, or after an identifier, separates values and is not a decimal point.
func (r *Redactor) findIdentifiers(s string, emails []Match) []Match {
	var ms []Match
	var balanced balancedScan
	lastEnd, e := -1, 0
	followsMatch := func(pos int) bool {
		for e < len(emails) && emails[e].End < pos {
			e++
		}
		return pos == lastEnd || (e < len(emails) && emails[e].End == pos)
	}
	for i := 0; i < len(s); {
		if !isASCIIAlnum(s[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && isASCIIAlnum(s[j]) {
			j++
		}
		if mayStart, escaped := mayStartIdentifier(s, i, j), followsEscape(s, i); mayStart || escaped {
			var c candidate
			var ok bool
			if mayStart {
				if allowed, decimal := leftBoundary(s, i, followsMatch(i-1)); allowed {
					c, ok = r.candidateAt(s, i, decimal)
				}
			}
			if !ok && escaped && mayStartIdentifier(s, i+1, j) {
				c, ok = r.candidateAt(s, i+1, false)
			}
			if ok && (r.policy == Strict || balanced.redacts(s, c, &ms)) {
				ms = append(ms, c.Match)
				lastEnd = c.End
				i = c.End
				continue
			}
		}
		// No candidate can start inside a run of letters and digits: its left neighbor would not be a
		// boundary.
		i = j
	}
	if balanced.reachedBack {
		slices.SortFunc(ms, compareOffsets)
	}
	return ms
}

// A candidate has a recognized shape, clean boundaries and valid check digits. Whether it is redacted is up
// to the policy.
type candidate struct {
	Match
	formatted bool
}

// valueRun is the current run of valid values of one entity during a scan, each at most contextWindow bytes after
// the previous one. A run is how the values of a list share the context of one keyword; a value of the other
// entity in between does not end it, so that mixed columns under "CPF/CNPJ" stay covered.
type valueRun struct {
	end     int     // where the last value of the run ends
	context bool    // whether the last value of the run has context
	kept    []Match // values of the run that were not redacted
}

// balancedScan is what the Balanced policy remembers during a scan.
type balancedScan struct {
	cpf, cnpj   valueRun
	reachedBack bool // whether any value was added to the matches out of order
}

// redacts applies the Balanced policy to c. A keyword after c also covers the values kept earlier in its run,
// which are added to ms out of order.
func (b *balancedScan) redacts(s string, c candidate, ms *[]Match) bool {
	run, other, keyword := &b.cpf, &b.cnpj, cpfKeyword
	if c.Entity == CNPJ {
		run, other, keyword = &b.cnpj, &b.cpf, cnpjKeyword
	}
	if c.Start < run.end || c.Start-run.end > contextWindow {
		run.context, run.kept = false, run.kept[:0]
	}
	// A formatted value can start inside a value kept earlier, as in "123456789-09.ABC.DEF/0001-21". Only one
	// of them can be redacted, and the formatted one is the better evidence.
	for n := len(other.kept); n > 0 && other.kept[n-1].End > c.Start; n-- {
		other.kept = other.kept[:n-1]
	}
	run.end = c.End
	before := run.context || keywordBefore(s, c.Start, keyword)
	after := !before && keywordAfter(s, c.End, keyword)
	run.context = before || after
	if after && len(run.kept) > 0 {
		*ms = append(*ms, run.kept...)
		run.kept = run.kept[:0]
		b.reachedBack = true
	}
	if !run.context && !c.formatted {
		run.kept = append(run.kept, c.Match)
		return false
	}
	return true
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

// candidateAt reports the CNPJ or CPF candidate that starts at s[i], if any, applying shape, right boundary
// and check-digit rules in that order. The left boundary is up to the caller, which says with afterDecimal
// whether s[i] would continue a decimal or dotted number, which rules out an unformatted candidate.
func (r *Redactor) candidateAt(s string, i int, afterDecimal bool) (candidate, bool) {
	if !r.noCNPJ {
		if end, c, formatted, ok := scanCNPJ(s, i); ok && (formatted || !afterDecimal) && rightBoundary(s, end, formatted) && cnpj.Valid(c) {
			return candidate{Match{Entity: CNPJ, Start: i, End: end}, formatted}, true
		}
	}
	if !r.noCPF {
		if end, d, formatted, ok := scanCPF(s, i); ok && (formatted || !afterDecimal) && rightBoundary(s, end, formatted) && cpf.Valid(d) {
			return candidate{Match{Entity: CPF, Start: i, End: end}, formatted}, true
		}
	}
	return candidate{}, false
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

// scanCNPJ matches a CNPJ shape at s[i]: XX.XXX.XXX/XXXX-DD or XXXXXXXX/XXXX-DD without lowercase letters
// (formatted), or XXXXXXXX/XXXX-DD with lowercase letters and XXXXXXXXXXXXDD (unformatted), where X is an
// ASCII letter or digit and D an ASCII digit. It returns the
// end offset and the fourteen characters, with letters folded to uppercase.
func scanCNPJ(s string, i int) (end int, c [cnpj.Len]byte, formatted, ok bool) {
	if i+18 <= len(s) && s[i+2] == '.' && s[i+6] == '.' && s[i+10] == '/' && s[i+15] == '-' &&
		alnumInto(c[0:2], s[i:i+2]) && alnumInto(c[2:5], s[i+3:i+6]) && alnumInto(c[5:8], s[i+7:i+10]) &&
		alnumInto(c[8:12], s[i+11:i+15]) && digitsInto(c[12:14], s[i+16:i+18]) {
		return i + 18, c, true, true
	}
	if i+16 <= len(s) && s[i+8] == '/' && s[i+13] == '-' &&
		alnumInto(c[0:8], s[i:i+8]) && alnumInto(c[8:12], s[i+9:i+13]) && digitsInto(c[12:14], s[i+14:i+16]) {
		// Lowercase letters make this shape look like a URL path ("20261030/main-12"), so it then counts
		// as unformatted.
		return i + 16, c, !hasLowerASCII(s[i : i+16]), true
	}
	if i+14 <= len(s) && alnumInto(c[0:12], s[i:i+12]) && digitsInto(c[12:14], s[i+12:i+14]) {
		return i + 14, c, false, true
	}
	return 0, c, false, false
}

func hasLowerASCII(s string) bool {
	for i := range len(s) {
		if 'a' <= s[i] && s[i] <= 'z' {
			return true
		}
	}
	return false
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

// leftBoundary reports whether a candidate may start at s[i]: the previous rune must not be a letter or digit, in
// any script. decimal reports that it is a '.' that follows an ASCII digit, which would make an unformatted
// candidate the tail of a decimal or dotted number — unless dotFollowsMatch reports that the '.' comes right after
// a detected value, in which case it separates two values. (That exception also keeps the result stable: once the
// earlier value is replaced, the '.' no longer follows a digit.) A formatted candidate cannot be read as a number,
// so a '.' before it does not matter ("1.123.456.789-09" can be a numbered list).
func leftBoundary(s string, i int, dotFollowsMatch bool) (allowed, decimal bool) {
	if i == 0 {
		return true, false
	}
	r, size := utf8.DecodeLastRuneInString(s[:i])
	if isWordRune(r) {
		return false, false
	}
	return true, r == '.' && i-size > 0 && isASCIIDigit(s[i-size-1]) && !dotFollowsMatch
}

// followsEscape reports whether s[i] is the letter of an escape sequence written out as two characters, as in
// JSON strings and logs: a line break must not glue what follows it to its letter.
func followsEscape(s string, i int) bool {
	return i > 0 && s[i-1] == '\\' && (s[i] == 'n' || s[i] == 'r' || s[i] == 't')
}

// rightBoundary reports whether a candidate may end at s[end]: the next rune must not be a letter or digit, and for
// an unformatted candidate must not be a '.' followed by an ASCII digit.
func rightBoundary(s string, end int, formatted bool) bool {
	if end == len(s) {
		return true
	}
	r, size := utf8.DecodeRuneInString(s[end:])
	if isWordRune(r) {
		return false
	}
	decimalHead := !formatted && r == '.' && end+size < len(s) && isASCIIDigit(s[end+size])
	return !decimalHead
}

// keywordAt returns the length of keyword (lowercase ASCII) at the start of s, in any case and possibly with
// dots between its letters ("C.P.F"), or 0 if it is not there.
func keywordAt(s, keyword string) int {
	k := 0
	for l := range len(s) {
		c := lowerASCII(s[l])
		if c == keyword[k] {
			k++
			if k == len(keyword) {
				return l + 1
			}
		} else if c != '.' || k == 0 {
			return 0
		}
	}
	return 0
}

// containsKeyword reports whether keyword occurs anywhere in s.
func containsKeyword(s, keyword string) bool {
	for j := range len(s) {
		if lowerASCII(s[j]) == keyword[0] && keywordAt(s[j:], keyword) > 0 {
			return true
		}
	}
	return false
}

// keywordBefore reports whether keyword occurs in the contextWindow bytes before start.
func keywordBefore(s string, start int, keyword string) bool {
	return containsKeyword(s[max(0, start-contextWindow):start], keyword)
}

// keywordAfter reports whether keyword is about the value that ends at s[end]. It looks in the next
// contextWindow bytes, up to the first digit (another number is closer to the keyword), line break (real or
// written out) or sentence end, for the keyword as a word of its own that does not label what comes next. The rule is stricter than keywordBefore because labels precede values: a keyword after a
// value is often the label of the next field, as in "id=12345678909 cpf=98765432100".
func keywordAfter(s string, end int, keyword string) bool {
	window := s[end:min(len(s), end+contextWindow)]
	for j := range len(window) {
		switch b := window[j]; {
		case lowerASCII(b) == keyword[0]:
			if n := keywordAt(window[j:], keyword); n > 0 {
				to := end + j + n
				if to < len(s) && lowerASCII(s[to]) == 's' {
					to++ // plural
				}
				if !touchesWord(s, end+j, to) && !labelsNext(s, to) {
					return true
				}
			}
		case isASCIIDigit(b), b == '\n', b == '\r', b == '.', b == ';', b == '!', b == '?':
			return false
		case b == '\\' && j+1 < len(window) && (window[j+1] == 'n' || window[j+1] == 'r'):
			return false
		}
	}
	return false
}

// touchesWord reports whether s[from:to] has a letter, digit, combining mark or '_' right next to it. The letter
// of a written-out escape, as in "12345678909\tCPF", does not count.
func touchesWord(s string, from, to int) bool {
	before, size := utf8.DecodeLastRuneInString(s[:from])
	if size > 0 && followsEscape(s, from-size) {
		before = ' '
	}
	after, _ := utf8.DecodeRuneInString(s[to:])
	return before == '_' || after == '_' || isWordRune(before) || isWordRune(after)
}

// labelsNext reports whether what follows a keyword at s[i] makes it a label: a ':', '=' or digit, possibly
// after spaces, quotes (escaped or not) and opening punctuation ("cpf: …", `"cpf":…`, `\"cpf\":…`, "cpf=…",
// "cpf 123…", "cpf (123…").
func labelsNext(s string, i int) bool {
	for limit := min(len(s), i+contextWindow); i < limit; i++ {
		switch b := s[i]; {
		case b == ':' || b == '=' || isASCIIDigit(b):
			return true
		case strings.IndexByte(" \t\"'\\-([#", b) < 0:
			return false
		}
	}
	return false
}

// resolveOverlaps sorts ms by offset and drops every match that overlaps an earlier one. Between matches
// that start at the same offset, the longest wins. An address whose local part reaches back into an identifier
// ("11.223.344/5566-13.ana@example.com") is not dropped: it starts after the identifier instead.
func resolveOverlaps(s string, ms []Match) []Match {
	slices.SortFunc(ms, compareOffsets)
	out := ms[:0]
	end := 0
	for _, m := range ms {
		if m.Start < end {
			if m.Entity != Email {
				continue
			}
			start, ok := localStart(s, end, strings.LastIndexByte(s[:m.End], '@'))
			if !ok {
				continue
			}
			m.Start = start
		}
		out = append(out, m)
		end = m.End
	}
	return out
}

func compareOffsets(a, b Match) int {
	if a.Start != b.Start {
		return cmp.Compare(a.Start, b.Start)
	}
	return cmp.Compare(b.End, a.End)
}

// isWordRune reports whether r continues a word: a letter, digit or combining mark in any script. The
// Portuguese ordinal indicators ª and º are letters in Unicode but mark abbreviations such as "nº", so
// they are boundaries.
func isWordRune(r rune) bool {
	if r < utf8.RuneSelf {
		return isASCIIAlnumRune(r)
	}
	if r == 'ª' || r == 'º' {
		return false
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
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
