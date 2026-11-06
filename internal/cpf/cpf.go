// Package cpf validates the check digits of CPF numbers (Cadastro de Pessoas Físicas).
//
// It implements validation only. Formatting, boundaries, context and the decision to redact belong to the
// detection policy in package redactbr.
//
// The algorithm is the one the Receita Federal documents in the e-Financeira filing manual (Anexo II,
// rule REGRA_VALIDA_CPF; see docs/spec/v0.1.md for the versions and URLs) and implements in the routine
// TestaCPF of its public CPF status page
// (https://servicos.receita.fazenda.gov.br/servicos/cpf/consultasituacao/funcoes03.js, accessed
// 2026-11-06). In the form used here, the first check digit weighs the nine base digits 10 down to 2, the
// second weighs the first ten digits 11 down to 2, and a remainder of 0 or 1 modulo 11 yields 0 (otherwise
// 11 minus the remainder). Like TestaCPF, the all-zero number is rejected and other repeated-digit numbers
// are not. Both sources are official technical material, not the regulation that governs the CPF.
package cpf

// Len is the number of digits in a CPF.
const Len = 11

// BaseLen is the number of digits before the two check digits.
const BaseLen = 9

// Valid reports whether d holds 11 ASCII digits whose last two are the check digits of the first nine.
// The all-zero number is not valid.
func Valid(d [Len]byte) bool {
	var base [BaseLen]byte
	copy(base[:], d[:BaseLen])
	dv1, dv2, ok := CheckDigits(base)
	if !ok || d[9] != dv1 || d[10] != dv2 {
		return false
	}
	for _, c := range d {
		if c != '0' {
			return true
		}
	}
	return false
}

// CheckDigits returns the two check digits, as ASCII digits, for a base of nine ASCII digits.
// ok is false if base contains anything other than ASCII digits.
func CheckDigits(base [BaseLen]byte) (dv1, dv2 byte, ok bool) {
	sum1, sum2 := 0, 0
	for i, c := range base {
		if c < '0' || c > '9' {
			return 0, 0, false
		}
		v := int(c - '0')
		sum1 += v * (10 - i) // weights 10..2
		sum2 += v * (11 - i) // weights 11..3; the first check digit gets weight 2 below
	}
	d1 := checkDigit(sum1)
	d2 := checkDigit(sum2 + d1*2)
	return asciiDigits[d1], asciiDigits[d2], true
}

const asciiDigits = "0123456789"

func checkDigit(sum int) int {
	if r := sum % 11; r >= 2 {
		return 11 - r
	}
	return 0
}
