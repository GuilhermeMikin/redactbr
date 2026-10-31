package cnpj

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func chars(t testing.TB, s string) [Len]byte {
	t.Helper()
	var c [Len]byte
	if len(s) != Len {
		t.Fatalf("test fixture has %d characters, want %d", len(s), Len)
	}
	copy(c[:], s)
	return c
}

func unmask(s string) string {
	return strings.NewReplacer(".", "", "/", "", "-", "").Replace(s)
}

// Official reference vectors. They are published by the Receita Federal in "Arquivos de referência para
// Cálculo do DV do CNPJ Alfanumérico" (codigos-cnpj.zip: src/java/TesterCalculo.java,
// src/java/TesterValidacao.java and src/typescript/test.ts) and in "Perguntas e respostas — CNPJ
// alfanumérico" (questions 14, 17 and 23), https://www.gov.br/receitafederal/pt-br/centrais-de-conteudo/
// publicacoes/documentos-tecnicos/cnpj, accessed 2026-10-30. CNPJ numbers are public registry data; these
// are used here only because the Receita Federal publishes them as test vectors.
var officialCheckDigits = []struct{ base, dv string }{
	{"12ABC34501DE", "35"},
	{"1345C3A50001", "06"},
	{"R55231B30007", "57"},
	{"90.021.382/0001", "22"},
	{"90.024.778/0001", "23"},
	{"90.025.108/0001", "21"},
	{"90.025.255/0001", "00"},
	{"90.024.420/0001", "09"},
	{"000000000001", "91"},
	{"12.ABC.345/01DE", "35"},
}

var officialValid = []string{
	"12ABC34501DE35",
	"1345C3A5000106",
	"R55231B3000757",
	"12.ABC.345/01DE-35",
	"90.021.382/0001-22",
	"90.024.778/0001-23",
	"90.025.108/0001-21",
	"90.025.255/0001-00",
	"90.024.420/0001-09",
	"90.024.781/0001-47",
	"04.740.714/0001-97",
	"44.108.058/0001-29",
	"90.024.780/0001-00",
	"90.024.779/0001-78",
	"00000000000191",
	"ABCDEFGHIJKL80",
	"12.345.678/0001-95", // FAQ question 17
	"12.345.678/000A-08", // FAQ question 23, example 3
	"AA345678/000A-29",   // FAQ question 23, example 2
}

var officialInvalid = []struct{ in, why string }{
	{"R55231B3000700", "wrong check digits"},
	{"1345c3A5000106", "lowercase letter"},
	{"12.ABc.345/01DE-35", "lowercase letter"},
	{"90.025.108/000101", "wrong check digits"},
	{"00000000000192", "wrong check digits"},
	{"ABCDEFGHIJKL81", "wrong check digits"},
	{"0000000000019L", "letter as second check digit"},
	{"000000000001P1", "letter as first check digit"},
	{"00000000000000", "all zeros"},
	{"00.000.000/0000-00", "all zeros"},
	// FAQ question 23, example 1, prints 29 as the check digits of AA345678/0003, but the published
	// algorithm yields 86 (the same base with order 000A does yield 29, example 2). We follow the
	// algorithm and record the discrepancy in docs/spec/v0.1.md.
	{"AA345678/0003-29", "FAQ example whose printed check digits do not match the algorithm"},
}

func TestCheckDigitsOfficialVectors(t *testing.T) {
	for _, v := range officialCheckDigits {
		var base [BaseLen]byte
		b := unmask(v.base)
		if len(b) != BaseLen {
			t.Fatalf("fixture base has %d characters", len(b))
		}
		copy(base[:], b)
		dv1, dv2, ok := CheckDigits(base)
		if !ok || string([]byte{dv1, dv2}) != v.dv {
			t.Errorf("CheckDigits(%s) = %c%c (ok %v), want %s", v.base, dv1, dv2, ok, v.dv)
		}
	}
}

func TestValidOfficialVectors(t *testing.T) {
	for _, s := range officialValid {
		if !Valid(chars(t, unmask(s))) {
			t.Errorf("Valid(%s) = false, want true", s)
		}
	}
	for _, v := range officialInvalid {
		if Valid(chars(t, unmask(v.in))) {
			t.Errorf("Valid(%s) = true, want false (%s)", v.in, v.why)
		}
	}
}

func TestValidRejectsMalformed(t *testing.T) {
	for _, s := range []string{
		"12ABC34501DE3 ",
		"12ABC34501D-35",
		"$0123456789A00",
		"0123456?789A00",
		"12ABC345\x0001DE35",
		"12ÀBC34501DE35"[:Len], // first byte of a two-byte UTF-8 letter
	} {
		var c [Len]byte
		copy(c[:], s)
		if Valid(c) {
			t.Errorf("Valid accepted a malformed input %q", s)
		}
	}
}

// tsReference is an independent port of the single-loop formulation in the Receita Federal's TypeScript
// reference routine (src/typescript/cnpj.ts): both sums are accumulated in one pass using the weight table
// 6,5,4,3,2,9,8,7,6,5,4,3,2 shifted by one position.
func tsReference(s string) bool {
	if len(s) != Len || s == "00000000000000" {
		return false
	}
	for i := range BaseLen {
		c := s[i]
		if !('0' <= c && c <= '9') && !('A' <= c && c <= 'Z') {
			return false
		}
	}
	if s[12] < '0' || s[12] > '9' || s[13] < '0' || s[13] > '9' {
		return false
	}
	pesos := [13]int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	s1, s2 := 0, 0
	for i := range BaseLen {
		v := int(s[i]) - '0'
		s1 += v * pesos[i+1]
		s2 += v * pesos[i]
	}
	dv := func(sum int) int {
		if sum%11 < 2 {
			return 0
		}
		return 11 - sum%11
	}
	d1 := dv(s1)
	s2 += d1 * pesos[12]
	d2 := dv(s2)
	return int(s[12]-'0') == d1 && int(s[13]-'0') == d2
}

const baseAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

func TestValidMatchesTypeScriptReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for range 20000 {
		var c [Len]byte
		for i := range BaseLen {
			c[i] = baseAlphabet[rng.IntN(len(baseAlphabet))]
		}
		c[12], c[13] = byte('0'+rng.IntN(10)), byte('0'+rng.IntN(10))
		if rng.IntN(2) == 0 {
			var base [BaseLen]byte
			copy(base[:], c[:BaseLen])
			c[12], c[13], _ = CheckDigits(base)
		}
		if got, want := Valid(c), tsReference(string(c[:])); got != want {
			t.Fatalf("Valid disagrees with the reference routine (got %v, want %v)", got, want)
		}
	}
}

func TestEveryWrongCheckDigitIsRejected(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	for range 2000 {
		var base [BaseLen]byte
		for i := range base {
			base[i] = baseAlphabet[rng.IntN(len(baseAlphabet))]
		}
		dv1, dv2, _ := CheckDigits(base)
		var c [Len]byte
		copy(c[:], base[:])
		for pos, right := range map[int]byte{12: dv1, 13: dv2} {
			for d := byte('0'); d <= '9'; d++ {
				c[12], c[13] = dv1, dv2
				c[pos] = d
				if Valid(c) != (d == right && string(c[:]) != "00000000000000") {
					t.Fatalf("check digit %d set to %q: unexpected validity", pos-11, d)
				}
			}
		}
	}
}

func FuzzValid(f *testing.F) {
	for _, s := range []string{"12ABC34501DE35", "00000000000000", "1345c3A5000106", "ABCDEFGHIJKL80", "0000000000019L"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var c [Len]byte
		copy(c[:], s)
		want := len(s) == Len && tsReference(s)
		if got := Valid(c) && len(s) == Len; got != want {
			t.Fatalf("Valid disagrees with the reference routine for a %d-byte input", len(s))
		}
	})
}
