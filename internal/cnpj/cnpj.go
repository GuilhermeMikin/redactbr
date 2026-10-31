// Package cnpj validates the check digits of CNPJ numbers (Cadastro Nacional da Pessoa Jurídica), in both
// the numeric and the alphanumeric format.
//
// It implements validation only. Formatting, case folding, boundaries, context and the decision to redact
// belong to the detection policy in package redactbr.
//
// Sources (accessed 2026-10-30): Receita Federal, "Cálculo dos dígitos verificadores de CNPJ alfanumérico"
// (https://www.gov.br/receitafederal/pt-br/centrais-de-conteudo/publicacoes/documentos-tecnicos/cnpj),
// its reference routines (codigos-cnpj.zip on the same page) and "Perguntas e respostas — CNPJ
// alfanumérico", question 14. A CNPJ has twelve characters (0-9 or uppercase A-Z) followed by two numeric
// check digits. Each character is worth its ASCII code minus 48 ('0'..'9' are 0..9, 'A'..'Z' are 17..42);
// weights 2..9 are applied from right to left, restarting after the eighth character; a remainder of 0 or 1
// modulo 11 yields 0, otherwise 11 minus the remainder. The second check digit repeats the calculation over
// the twelve characters plus the first check digit. Numeric CNPJs are the special case with digits only.
// Like the reference routines, the all-zero number and lowercase letters are rejected.
package cnpj

// Len is the number of characters in a CNPJ.
const Len = 14

// BaseLen is the number of characters before the two check digits.
const BaseLen = 12

// weights for the first check digit; the second uses 6 followed by these, over thirteen characters.
var weights = [BaseLen]int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}

// Valid reports whether c holds twelve characters in 0-9 or A-Z followed by the two ASCII check digits
// that the official algorithm computes for them. The all-zero number is not valid.
func Valid(c [Len]byte) bool {
	var base [BaseLen]byte
	copy(base[:], c[:BaseLen])
	dv1, dv2, ok := CheckDigits(base)
	if !ok || c[12] != dv1 || c[13] != dv2 {
		return false
	}
	for _, b := range c {
		if b != '0' {
			return true
		}
	}
	return false
}

// CheckDigits returns the two check digits, as ASCII digits, for a twelve-character base.
// ok is false if base contains anything other than 0-9 and uppercase A-Z.
func CheckDigits(base [BaseLen]byte) (dv1, dv2 byte, ok bool) {
	sum1, sum2 := 0, 0
	for i, c := range base {
		if !isBaseChar(c) {
			return 0, 0, false
		}
		v := int(c) - '0'
		sum1 += v * weights[i]
		if i == 0 {
			sum2 += v * 6
		} else {
			sum2 += v * weights[i-1]
		}
	}
	d1 := checkDigit(sum1)
	d2 := checkDigit(sum2 + d1*2)
	return asciiDigits[d1], asciiDigits[d2], true
}

func isBaseChar(c byte) bool {
	return ('0' <= c && c <= '9') || ('A' <= c && c <= 'Z')
}

const asciiDigits = "0123456789"

func checkDigit(sum int) int {
	if r := sum % 11; r >= 2 {
		return 11 - r
	}
	return 0
}
