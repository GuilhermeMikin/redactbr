// Command slog redacts log records with a RedactBR handler in front of a standard JSON handler. The identifiers
// are generated test values, not real ones. The output has one line per record.
package main

import (
	"errors"
	"log"
	"log/slog"
	"os"

	"github.com/GuilhermeMikin/redactbr"
)

type customer struct {
	Name string
	CPF  string
}

func main() {
	r, err := redactbr.New()
	if err != nil {
		log.Fatal(err)
	}
	logger := slog.New(r.NewSlogHandler(slog.NewJSONHandler(os.Stdout, nil)))

	// In the message, and as the value of a field named after the entity.
	logger.Info("signup from ana.silva@example.com", "cpf", "12345678909", "cnpj", "AB12CD34EF5602")

	// A number with valid check digits stays: "order" is not context for a CPF.
	logger.Info("order created", "order", "98765432100", "items", 3)

	// Errors and structs are logged as redacted text.
	logger.Error("charge failed", "err", errors.New("CPF 123.456.789-09 is locked"), "customer", customer{"Ana", "98765432100"})

	// Groups count as context too, and attributes added with With are redacted once.
	logger.With(slog.Group("cpf", "value", "12345678909")).Info("lookup done")
}
