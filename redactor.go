package redactbr

import (
	"errors"
	"fmt"
	"strings"
)

// Entity identifies a kind of personal data that RedactBR detects. Its value is the label used in
// replacements and tokens.
type Entity string

const (
	// CPF is the Brazilian individual taxpayer number (Cadastro de Pessoas Físicas).
	CPF Entity = "CPF"
	// CNPJ is the Brazilian legal-entity number (Cadastro Nacional da Pessoa Jurídica), in the numeric or
	// the alphanumeric format.
	CNPJ Entity = "CNPJ"
	// Email is an email address.
	Email Entity = "EMAIL"
)

// Policy decides which candidates with valid check digits are redacted. Validation decides whether a
// candidate is a well-formed identifier; the policy decides whether a valid candidate is redacted.
type Policy int

const (
	// Balanced redacts formatted identifiers (such as 123.456.789-09) whose check digits are valid, and
	// unformatted ones (such as 12345678909) only when the entity's keyword ("cpf" or "cnpj", in any case)
	// appears in the 32 bytes before them. It is the default.
	Balanced Policy = iota
	// Strict redacts every candidate whose check digits are valid and whose boundaries are clean, formatted
	// or not, with or without context. It accepts more false positives, such as order numbers or
	// timestamps that happen to have valid check digits.
	Strict
)

// String returns "balanced" or "strict".
func (p Policy) String() string {
	switch p {
	case Balanced:
		return "balanced"
	case Strict:
		return "strict"
	default:
		return fmt.Sprintf("Policy(%d)", int(p))
	}
}

// DefaultMaxSessionValues is the default capacity of a Session: the number of distinct values it can
// tokenize, plus token-like strings it has reserved, before Session.Redact returns ErrSessionFull.
const DefaultMaxSessionValues = 10000

// Match is one detected value: text[Start:End] is the value. It deliberately does not carry the value, so
// a Match can be logged without leaking it.
type Match struct {
	Entity Entity
	Start  int // byte offset of the first byte
	End    int // byte offset just past the last byte
}

// String formats the match as ENTITY[start:end], without the value.
func (m Match) String() string {
	return fmt.Sprintf("%s[%d:%d]", m.Entity, m.Start, m.End)
}

// Redactor detects and redacts personal data in text. It holds only configuration, so a Redactor is safe
// for concurrent use by multiple goroutines. The zero value is ready to use and behaves like New() without
// options: every entity, the Balanced policy and the default session capacity.
type Redactor struct {
	// The fields record what is turned off, so that a zero Redactor detects everything instead of nothing.
	policy    Policy
	noCPF     bool
	noCNPJ    bool
	noEmail   bool
	maxValues int // 0 means DefaultMaxSessionValues
}

// Option configures a Redactor.
type Option func(*config)

type config struct {
	policy    Policy
	entities  []Entity
	maxValues int
}

// WithPolicy sets the detection policy. The default is Balanced.
func WithPolicy(p Policy) Option {
	return func(c *config) { c.policy = p }
}

// WithEntities sets the entities to detect. The default is CPF, CNPJ and Email. Passing no entity is an
// error: a redactor that detects nothing would silently pass all data through.
func WithEntities(entities ...Entity) Option {
	return func(c *config) { c.entities = append([]Entity{}, entities...) }
}

// WithMaxSessionValues sets the capacity of the sessions created by the Redactor (see NewSession). The
// default is DefaultMaxSessionValues; n must be at least 1.
func WithMaxSessionValues(n int) Option {
	return func(c *config) { c.maxValues = n }
}

// New returns a Redactor configured by opts. It returns an error, and no Redactor, if the configuration is
// invalid.
func New(opts ...Option) (*Redactor, error) {
	c := config{
		policy:    Balanced,
		entities:  []Entity{CPF, CNPJ, Email},
		maxValues: DefaultMaxSessionValues,
	}
	for _, opt := range opts {
		opt(&c)
	}
	r := &Redactor{policy: c.policy, maxValues: c.maxValues, noCPF: true, noCNPJ: true, noEmail: true}
	switch c.policy {
	case Balanced, Strict:
	default:
		return nil, fmt.Errorf("redactbr: unknown policy %d", int(c.policy))
	}
	if len(c.entities) == 0 {
		return nil, errors.New("redactbr: no entities selected")
	}
	for _, e := range c.entities {
		switch e {
		case CPF:
			r.noCPF = false
		case CNPJ:
			r.noCNPJ = false
		case Email:
			r.noEmail = false
		default:
			return nil, fmt.Errorf("redactbr: unknown entity %q", string(e))
		}
	}
	if c.maxValues < 1 {
		return nil, fmt.Errorf("redactbr: session capacity must be at least 1, got %d", c.maxValues)
	}
	return r, nil
}

// capacity returns the session capacity, applying the default for a zero Redactor.
func (r *Redactor) capacity() int {
	if r.maxValues == 0 {
		return DefaultMaxSessionValues
	}
	return r.maxValues
}

// Find returns the values in text that the Redactor would redact, in order and without overlaps.
func (r *Redactor) Find(text string) []Match {
	return r.find(text)
}

// Redact returns text with every detected value replaced by its entity label in angle brackets, such as
// <CPF>, <CNPJ> or <EMAIL>. The replacement is irreversible; use a Session for reversible tokens.
func (r *Redactor) Redact(text string) string {
	ms := r.find(text)
	if len(ms) == 0 {
		return text
	}
	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for _, m := range ms {
		b.WriteString(text[last:m.Start])
		b.WriteByte('<')
		b.WriteString(string(m.Entity))
		b.WriteByte('>')
		last = m.End
	}
	b.WriteString(text[last:])
	return b.String()
}
