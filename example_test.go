package redactbr_test

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/GuilhermeMikin/redactbr"
)

// The identifiers in these examples are generated test values.

func ExampleRedactor_Redact() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(r.Redact("CPF 123.456.789-09, CNPJ AB.12C.D34/EF56-02, email ana.silva@example.com"))
	// Output: CPF <CPF>, CNPJ <CNPJ>, email <EMAIL>
}

func ExampleWithPolicy() {
	text := "order 98765432100 / cpf 12345678909"
	balanced, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	strict, err := redactbr.New(redactbr.WithPolicy(redactbr.Strict))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(balanced.Redact(text)) // the "cpf" keyword labels the second number, not the order number
	fmt.Println(strict.Redact(text))
	// Output:
	// order 98765432100 / cpf <CPF>
	// order <CPF> / cpf <CPF>
}

func ExampleRedactor_Find() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	for _, m := range r.Find("CPF 123.456.789-09 / ana.silva@example.com") {
		fmt.Println(m) // offsets only: safe to log
	}
	// Output:
	// CPF[4:18]
	// EMAIL[21:42]
}

func ExampleSession() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	session := r.NewSession()
	prompt, err := session.Redact("My CPF is 123.456.789-09")
	if err != nil {
		log.Fatal(err) // fail closed: send nothing
	}
	token := strings.TrimPrefix(prompt, "My CPF is ")   // random, such as <CPF_k7q2xm9pb3>
	answer := "Thanks, CPF " + token + " is confirmed." // what a model might answer
	fmt.Println(session.Restore(answer))
	fmt.Println(session.Restore("<CPF_0000000000> is not a token this session issued."))
	// Output:
	// Thanks, CPF 123.456.789-09 is confirmed.
	// <CPF_0000000000> is not a token this session issued.
}

func ExampleSession_RestoreWriter() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	session := r.NewSession()
	prompt, err := session.Redact("email ana.silva@example.com")
	if err != nil {
		log.Fatal(err)
	}
	token := strings.TrimPrefix(prompt, "email ")
	answer := "I will write to " + token + " today."

	w := session.RestoreWriter(os.Stdout)
	for i := 0; i < len(answer); i += 5 { // chunks that split the token
		if _, err := w.Write([]byte(answer[i:min(i+5, len(answer))])); err != nil {
			log.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
	// Output: I will write to ana.silva@example.com today.
}
