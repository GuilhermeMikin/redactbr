package main

import (
	"regexp"
	"strings"
)

// An outcome says what happened to one token in one answer. Only outcomeExact and outcomeMixed restore the
// value; every other outcome leaves the reader with a broken token or nothing.
type outcome string

const (
	outcomeExact     outcome = "exact"     // present byte for byte
	outcomeMixed     outcome = "mixed"     // present byte for byte, and changed somewhere else in the answer
	outcomeCase      outcome = "case"      // <cpf_K7Q2XM9PB3>
	outcomeEscaped   outcome = "escaped"   // &lt;CPF_k7q2xm9pb3&gt;, \u003cCPF_k7q2xm9pb3\u003e
	outcomeSpacing   outcome = "spacing"   // < CPF_k7q2xm9pb3 >
	outcomeSeparator outcome = "separator" // <CPF-k7q2xm9pb3>, <CPF k7q2xm9pb3>
	outcomeBrackets  outcome = "brackets"  // CPF_k7q2xm9pb3, [CPF_k7q2xm9pb3]
	outcomeOther     outcome = "other"     // the id is there, in some other form
	outcomeIDEdited  outcome = "id_edited" // an id within two edits of the real one
	outcomeMissing   outcome = "missing"   // nothing recognizable
)

// allOutcomes lists the outcomes in the order reports show them.
var allOutcomes = []outcome{
	outcomeExact, outcomeMixed, outcomeCase, outcomeEscaped, outcomeSpacing, outcomeSeparator, outcomeBrackets,
	outcomeOther, outcomeIDEdited, outcomeMissing,
}

// tokenPattern restates the token grammar of docs/spec/v0.1.md §12.1; the harness uses only the public API.
var tokenPattern = regexp.MustCompile(`<(?:CPF|CNPJ|EMAIL)_[0-9a-hjkmnp-tv-z]{10}>`)

// classify says what happened to token in answer.
func classify(answer, token string) outcome {
	entity, id, _ := strings.Cut(token[1:len(token)-1], "_")
	entity = strings.ToLower(entity)
	lower := strings.ToLower(answer)
	exact, mentions := strings.Count(answer, token), strings.Count(lower, id)
	switch {
	case exact > 0 && mentions > exact:
		return outcomeMixed
	case exact > 0:
		return outcomeExact
	case mentions == 0:
		if hasNearID(lower, id) {
			return outcomeIDEdited
		}
		return outcomeMissing
	}
	bare := entity + "_" + id
	switch {
	case strings.Contains(lower, "<"+bare+">"):
		return outcomeCase
	case strings.Contains(lower, "&lt;"+bare), strings.Contains(lower, `\u003c`+bare), strings.Contains(lower, `\x3c`+bare):
		return outcomeEscaped
	case regexp.MustCompile(`<\s*` + entity + `\s*_\s*` + id + `\s*>`).MatchString(lower):
		return outcomeSpacing
	case regexp.MustCompile(`<\s*` + entity + `[^0-9a-z_]+` + id + `\s*>`).MatchString(lower):
		return outcomeSeparator
	case strings.Contains(lower, bare):
		return outcomeBrackets
	}
	return outcomeOther
}

// hasNearID reports whether a run of letters and digits in lower, of about the length of id, is within two
// edits of it. Words of ordinary text are not: the ids are random.
func hasNearID(lower, id string) bool {
	isRune := func(r rune) bool { return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' }
	for _, word := range strings.FieldsFunc(lower, func(r rune) bool { return !isRune(r) }) {
		if len(word) >= len(id)-2 && len(word) <= len(id)+2 && editDistance(word, id) <= 2 {
			return true
		}
	}
	return false
}

// editDistance is the Levenshtein distance between two ASCII strings.
func editDistance(a, b string) int {
	row := make([]int, len(b)+1)
	for j := range row {
		row[j] = j
	}
	for i := 1; i <= len(a); i++ {
		diagonal := row[0]
		row[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			diagonal, row[j] = row[j], min(row[j]+1, row[j-1]+1, diagonal+cost)
		}
	}
	return row[len(b)]
}

// invented returns the well-formed tokens in answer that the session did not issue.
func invented(answer string, issued map[string]bool) []string {
	var out []string
	for _, tok := range tokenPattern.FindAllString(answer, -1) {
		if !issued[tok] {
			out = append(out, tok)
		}
	}
	return out
}
