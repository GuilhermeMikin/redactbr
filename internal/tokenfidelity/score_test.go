package main

import (
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	const tok = "<CPF_k7q2xm9pb3>"
	tests := []struct {
		name, answer string
		want         outcome
	}{
		{"exact", "CPF: " + tok + ".", outcomeExact},
		{"twice", tok + " and " + tok, outcomeExact},
		{"exact and changed", tok + " and " + strings.ToUpper(tok), outcomeMixed},
		{"exact and without brackets", tok + " or CPF_k7q2xm9pb3", outcomeMixed},
		{"uppercase", "<CPF_K7Q2XM9PB3>", outcomeCase},
		{"lowercase entity", "<cpf_k7q2xm9pb3>", outcomeCase},
		{"HTML entities", "&lt;CPF_k7q2xm9pb3&gt;", outcomeEscaped},
		{"JSON escape", `\u003cCPF_k7q2xm9pb3\u003e`, outcomeEscaped},
		{"spaces inside", "< CPF_k7q2xm9pb3 >", outcomeSpacing},
		{"space at the end", "<CPF_k7q2xm9pb3 >", outcomeSpacing},
		{"hyphen", "<CPF-k7q2xm9pb3>", outcomeSeparator},
		{"space as separator", "<CPF k7q2xm9pb3>", outcomeSeparator},
		{"no brackets", "CPF_k7q2xm9pb3", outcomeBrackets},
		{"square brackets", "[CPF_k7q2xm9pb3]", outcomeBrackets},
		{"double braces", "{{CPF_k7q2xm9pb3}}", outcomeBrackets},
		{"closing tag", "</CPF_k7q2xm9pb3>", outcomeBrackets},
		{"the id alone", "k7q2xm9pb3", outcomeOther},
		{"a letter wrong", "<CPF_k7q2xm9pb4>", outcomeIDEdited},
		{"a letter missing", "<CPF_k7q2xm9pb>", outcomeIDEdited},
		{"two letters swapped", "<CPF_k7q2xm9p3b>", outcomeIDEdited},
		{"another id", "<CPF_0000000000>", outcomeMissing},
		{"nothing", "Your data was received.", outcomeMissing},
		{"empty", "", outcomeMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.answer, tok); got != tt.want {
				t.Errorf("classify(%q) = %s, want %s", tt.answer, got, tt.want)
			}
		})
	}
}

func TestEditDistance(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"abc", "abc", 0}, {"abc", "abd", 1}, {"abc", "ab", 1}, {"ab", "abc", 1}, {"abcd", "acbd", 2}, {"", "abc", 3}} {
		if got := editDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestInvented(t *testing.T) {
	issued := map[string]bool{"<CPF_k7q2xm9pb3>": true}
	got := invented("<CPF_k7q2xm9pb3> <CPF_0000000000> <EMAIL_abcdefghjk> <CPF_1> <cpf_0000000000>", issued)
	if len(got) != 2 || got[0] != "<CPF_0000000000>" || got[1] != "<EMAIL_abcdefghjk>" {
		t.Errorf("invented = %v", got)
	}
}
