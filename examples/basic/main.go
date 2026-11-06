// Command basic redacts a support message with typed replacement under the default (Balanced) and the
// Strict policy, then tokenizes it reversibly with a session. The identifiers are generated test values,
// not real ones.
package main

import (
	"fmt"
	"log"

	"github.com/GuilhermeMikin/redactbr"
)

const input = "Customer CPF: 123.456.789-09. Supplier CNPJ AB.12C.D34/EF56-02 (alphanumeric format). " +
	"Contact: ana.silva@example.com. Their cpf 98765432100 was also mentioned. Order 11223344556613 shipped."

func main() {
	balanced, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	strict, err := redactbr.New(redactbr.WithPolicy(redactbr.Strict))
	if err != nil {
		log.Fatal(err)
	}

	section("Input", input)
	section("Typed replacement, Balanced (the default; unformatted numbers need a keyword such as \"cpf\" nearby)",
		balanced.Redact(input))
	section("Typed replacement, Strict (every value with valid check digits, including an order number that "+
		"happens to have valid CNPJ check digits)", strict.Redact(input))

	session := balanced.NewSession()
	redacted, err := session.Redact(input)
	if err != nil {
		log.Fatal(err) // fail closed: never fall back to the input
	}
	section("Redacted with a session (token ids are random per session)", redacted)
	section("Restored", session.Restore(redacted))

	fmt.Println("Matches (offsets only, never values):")
	for _, m := range balanced.Find(input) {
		fmt.Println(" ", m)
	}
}

func section(title, text string) {
	fmt.Printf("%s:\n  %s\n\n", title, text)
}
