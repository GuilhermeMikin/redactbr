package redactbr

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrSessionFull is returned by Session.Redact when tokenizing the text would exceed the session's
// capacity (see WithMaxSessionValues). The session is left unchanged.
var ErrSessionFull = errors.New("redactbr: session capacity exceeded")

// errTokenCollision means that no unused token could be generated; with crypto/rand it does not happen in
// practice.
var errTokenCollision = errors.New("redactbr: could not generate an unused token")

// maxTokenAttempts bounds token generation when a random id collides with a token already issued or
// reserved.
const maxTokenAttempts = 8

// A Session replaces detected values with reversible tokens such as <CPF_k7q2xm9pb3> and restores them
// later, typically in an LLM's answer. The same value gets the same token for the lifetime of the session,
// and only tokens the session issued are restored.
//
// A session is a trust scope: everything it has tokenized can be restored into any text passed to its
// Restore methods, so use one session per conversation or request, never one shared across users. A
// Session is safe for concurrent use by multiple goroutines.
type Session struct {
	r    *Redactor
	fill func([]byte) error // random source; crypto/rand outside tests

	mu       sync.RWMutex
	tokens   map[valueKey]string // normalized value -> token
	values   map[string]string   // token -> original text, as first seen
	reserved map[string]struct{} // well-formed tokens seen in input before this session issued them
}

type valueKey struct {
	entity Entity
	value  string
}

// NewSession returns an empty Session that uses r's detection settings and capacity.
func (r *Redactor) NewSession() *Session {
	return &Session{
		r:        r,
		fill:     cryptoFill,
		tokens:   make(map[valueKey]string),
		values:   make(map[string]string),
		reserved: make(map[string]struct{}),
	}
}

func cryptoFill(b []byte) error {
	_, err := rand.Read(b)
	return err
}

// Redact returns text with every detected value replaced by a session token. Values already seen by the
// session reuse their token; equal values written differently (123.456.789-09 and 12345678909) share one.
//
// Well-formed tokens that appear in text but were not issued by this session are reserved: the session
// will never issue them, so text that arrives looking like a token can never be restored into a value.
// Tokens the session did issue (for example in conversation history sent again) are left as they are.
//
// On error, Redact returns an empty string and leaves the session unchanged; it never returns text with
// values left in place.
func (s *Session) Redact(text string) (string, error) {
	ms := s.r.find(text)
	literals := tokenLiterals(text)
	if len(ms) == 0 && len(literals) == 0 {
		return text, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newReserved := make(map[string]struct{})
	for _, lit := range literals {
		if _, issued := s.values[lit]; !issued {
			if _, ok := s.reserved[lit]; !ok {
				newReserved[lit] = struct{}{}
			}
		}
	}
	type newValue struct {
		key      valueKey
		original string
	}
	var fresh []newValue
	keys := make([]valueKey, len(ms))
	pending := make(map[valueKey]string) // key -> token for values new in this call
	for i, m := range ms {
		k := valueKey{m.Entity, normalize(m.Entity, text[m.Start:m.End])}
		keys[i] = k
		if _, ok := s.tokens[k]; ok {
			continue
		}
		if _, ok := pending[k]; ok {
			continue
		}
		pending[k] = ""
		fresh = append(fresh, newValue{k, text[m.Start:m.End]})
	}
	if len(s.values)+len(s.reserved)+len(newReserved)+len(fresh) > s.r.maxValues {
		return "", ErrSessionFull
	}

	taken := func(tok string) bool {
		_, issued := s.values[tok]
		_, reserved := s.reserved[tok]
		_, reservedNow := newReserved[tok]
		return issued || reserved || reservedNow
	}
	generated := make(map[string]struct{}, len(fresh))
	for _, v := range fresh {
		tok, err := s.newToken(v.key.entity, func(t string) bool {
			_, dup := generated[t]
			return dup || taken(t)
		})
		if err != nil {
			return "", err
		}
		generated[tok] = struct{}{}
		pending[v.key] = tok
	}

	// Nothing can fail from here on.
	for lit := range newReserved {
		s.reserved[lit] = struct{}{}
	}
	for _, v := range fresh {
		tok := pending[v.key]
		s.tokens[v.key] = tok
		s.values[tok] = v.original
	}

	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for i, m := range ms {
		b.WriteString(text[last:m.Start])
		b.WriteString(s.tokens[keys[i]])
		last = m.End
	}
	b.WriteString(text[last:])
	return b.String(), nil
}

// newToken returns a random token for entity for which taken reports false.
func (s *Session) newToken(entity Entity, taken func(string) bool) (string, error) {
	var id [idLen]byte
	for range maxTokenAttempts {
		if err := s.fill(id[:]); err != nil {
			return "", fmt.Errorf("redactbr: reading randomness for a token: %w", err)
		}
		for i, b := range id {
			id[i] = idAlphabet[b&31] // 256 is a multiple of 32, so every symbol is equally likely
		}
		tok := "<" + string(entity) + "_" + string(id[:]) + ">"
		if !taken(tok) {
			return tok, nil
		}
	}
	return "", errTokenCollision
}

// Restore returns text with every token issued by this session replaced by the original value. Anything
// else — unknown, modified or forged tokens, tokens from other sessions, reserved token-like text — is
// left exactly as it is.
func (s *Session) Restore(text string) string {
	if strings.IndexByte(text, '<') < 0 {
		return text
	}
	s.mu.RLock()
	out, _ := restoreAppend(s.values, make([]byte, 0, len(text)), text, true)
	s.mu.RUnlock()
	return string(out)
}

// String describes the session without revealing any value or token, so that a session that ends up in a
// log or an error message does not leak what it holds.
func (s *Session) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("redactbr.Session{values: %d, reserved: %d}", len(s.values), len(s.reserved))
}

// GoString is like String, so that %#v does not print the session's contents either.
func (s *Session) GoString() string {
	return s.String()
}

// normalize returns the canonical form of a detected value: CPF and CNPJ without punctuation and with
// letters in uppercase; email with the domain in lowercase (the local part is case-sensitive).
func normalize(e Entity, v string) string {
	if e == Email {
		at := strings.LastIndexByte(v, '@')
		return v[:at+1] + strings.ToLower(v[at+1:])
	}
	b := make([]byte, 0, len(v))
	for i := range len(v) {
		if c := v[i]; isASCIIAlnum(c) {
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			b = append(b, c)
		}
	}
	return string(b)
}
