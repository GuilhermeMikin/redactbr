package cpf

import (
	"math/rand/v2"
	"testing"
)

// Every CPF in this file is generated from a simple, explainable base (sequential digits, digits of pi,
// single non-zero digits); none was collected. A generated number can still coincide with an assigned one.

func digits(t testing.TB, s string) [Len]byte {
	t.Helper()
	var d [Len]byte
	if len(s) != Len {
		t.Fatalf("test fixture has %d characters, want %d", len(s), Len)
	}
	copy(d[:], s)
	return d
}

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"sequential base", "12345678909", true},
		{"descending base", "98765432100", true},
		{"single digit base", "00000000191", true},
		{"pi digits", "31415926590", true},
		{"first check digit from remainder 0", "00000001406", true},
		{"first check digit from remainder 1", "00000000604", true},
		{"second check digit from remainder 0", "00000001910", true},
		{"second check digit from remainder 1", "00000001830", true},
		// Repeated digits pass the official routine; only the all-zero number is rejected by it.
		{"repeated ones", "11111111111", true},
		{"repeated nines", "99999999999", true},
		{"all zeros", "00000000000", false},
		{"wrong first check digit", "12345678919", false},
		{"wrong second check digit", "12345678908", false},
		{"swapped check digits", "12345678990", false},
		{"letter in base", "1234567890A", false},
		{"letter in check digits", "123456789A9", false},
		{"punctuation", "123.4567890", false},
		{"spaces", "1234567890 ", false},
		{"full-width digit", "\xef\xbc\x91" + "23456789", false}, // U+FF11 followed by 8 digits
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Valid(digits(t, tt.in)); got != tt.want {
				t.Errorf("Valid = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckDigitsRejectsNonDigits(t *testing.T) {
	for _, base := range []string{"12345678A", "1234 5678", "-12345678", "\x0012345678"} {
		var b [BaseLen]byte
		copy(b[:], base)
		if _, _, ok := CheckDigits(b); ok {
			t.Errorf("CheckDigits accepted a base with a non-digit (len %d)", len(base))
		}
	}
}

// receitaTestaCPF is an independent port of the Receita Federal page routine TestaCPF (see the package
// documentation). It uses a different formulation, Resto = (Soma*10) % 11 with 10 mapped to 0, so
// agreeing with it is evidence that Valid implements the same rule.
func receitaTestaCPF(s string) bool {
	if len(s) != Len {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	if s == "00000000000" {
		return false
	}
	check := func(n int) bool {
		soma := 0
		for i := 1; i <= n; i++ {
			soma += int(s[i-1]-'0') * (n + 2 - i)
		}
		resto := (soma * 10) % 11
		if resto == 10 || resto == 11 {
			resto = 0
		}
		return resto == int(s[n]-'0')
	}
	return check(9) && check(10)
}

func TestValidMatchesReceitaRoutine(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		var d [Len]byte
		for i := range d {
			d[i] = byte('0' + rng.IntN(10))
		}
		if rng.IntN(2) == 0 { // half of the cases get correct check digits
			var base [BaseLen]byte
			copy(base[:], d[:BaseLen])
			d[9], d[10], _ = CheckDigits(base)
		}
		if got, want := Valid(d), receitaTestaCPF(string(d[:])); got != want {
			t.Fatalf("Valid disagrees with the Receita routine (got %v, want %v)", got, want)
		}
	}
}

func TestEveryWrongCheckDigitIsRejected(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for range 2000 {
		var base [BaseLen]byte
		for i := range base {
			base[i] = byte('0' + rng.IntN(10))
		}
		dv1, dv2, _ := CheckDigits(base)
		var d [Len]byte
		copy(d[:], base[:])
		for pos, right := range map[int]byte{9: dv1, 10: dv2} {
			for c := byte('0'); c <= '9'; c++ {
				d[9], d[10] = dv1, dv2
				d[pos] = c
				if Valid(d) != (c == right && string(d[:]) != "00000000000") {
					t.Fatalf("check digit %d set to %q: unexpected validity", pos-8, c)
				}
			}
		}
	}
}

func FuzzValid(f *testing.F) {
	for _, s := range []string{"12345678909", "00000000000", "11111111111", "12345678908", "1234567890A"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var d [Len]byte
		copy(d[:], s)
		want := len(s) == Len && receitaTestaCPF(s)
		if got := Valid(d) && len(s) == Len; got != want {
			t.Fatalf("Valid disagrees with the Receita routine for a %d-byte input", len(s))
		}
	})
}
