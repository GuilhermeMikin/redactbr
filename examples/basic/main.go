// Command basic redacts a support message with typed replacement under the default (Balanced) and the
// Strict policy. The identifiers are generated test values, not real ones.
package main

import (
	"fmt"
	"log"

	"github.com/GuilhermeMikin/redactbr"
)

const input = "Customer CPF: 123.456.789-09. Supplier CNPJ AB.12C.D34/EF56-02 (new alphanumeric format). " +
	"Contact: ana.silva@example.com. cpf 98765432100 was also mentioned. Order 11223344556613 shipped."

func main() {
	balanced, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	strict, err := redactbr.New(redactbr.WithPolicy(redactbr.Strict))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Input:")
	fmt.Println(" ", input)
	fmt.Println()
	fmt.Println("Balanced (default): unformatted numbers need a keyword such as \"cpf\" before them")
	fmt.Println(" ", balanced.Redact(input))
	fmt.Println()
	fmt.Println("Strict: every value with valid check digits is redacted")
	fmt.Println(" ", strict.Redact(input))
	fmt.Println()
	fmt.Println("Matches (offsets only, never values):")
	for _, m := range balanced.Find(input) {
		fmt.Println(" ", m)
	}
}
