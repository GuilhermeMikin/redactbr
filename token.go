package redactbr

import (
	"bytes"
	"strings"
)

// Session tokens have the form
//
//	token  = "<" entity "_" id ">"
//	entity = "CPF" / "CNPJ" / "EMAIL"
//	id     = 10 characters from idAlphabet
//
// The id is drawn from crypto/rand for every new value, so a token carries no information about the value
// or the session, and guessing one issued by a session succeeds with probability about n/2^50 per guess,
// where n is the number of tokens of that entity the session has issued.

// idAlphabet is Crockford's base32 alphabet in lowercase: digits and letters without i, l, o and u.
const idAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

const (
	idLen = 10
	// maxTokenLen is the length of the longest token, "<EMAIL_" + id + ">". A streaming restorer holds
	// back at most maxTokenLen-1 bytes.
	maxTokenLen = len("<EMAIL_") + idLen + len(">")
)

var tokenPrefixes = [...]string{"<CPF_", "<CNPJ_", "<EMAIL_"}

type tokenStatus uint8

const (
	tokenNone     tokenStatus = iota // not a token
	tokenPartial                     // the input ends inside something that could still become a token
	tokenComplete                    // a well-formed token of the returned length
)

// parseToken reports whether b, which starts with '<', begins with a well-formed token.
func parseToken[T string | []byte](b T) (int, tokenStatus) {
	for _, p := range tokenPrefixes {
		if len(b) < len(p) {
			if string(b) == p[:len(b)] {
				return 0, tokenPartial
			}
			continue
		}
		if string(b[:len(p)]) != p {
			continue
		}
		k := len(p)
		for ; k < len(p)+idLen; k++ {
			if k == len(b) {
				return 0, tokenPartial
			}
			if strings.IndexByte(idAlphabet, b[k]) < 0 {
				return 0, tokenNone
			}
		}
		if k == len(b) {
			return 0, tokenPartial
		}
		if b[k] != '>' {
			return 0, tokenNone
		}
		return k + 1, tokenComplete
	}
	return 0, tokenNone
}

// tokenLiterals returns the distinct well-formed tokens that occur in s.
func tokenLiterals(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		j := strings.IndexByte(s[i:], '<')
		if j < 0 {
			break
		}
		i += j
		if n, st := parseToken(s[i:]); st == tokenComplete {
			out = append(out, s[i:i+n])
			i += n
			continue
		}
		i++
	}
	return out
}

// restoreAppend appends src to dst, replacing every token found in values with its value. Text that is
// not a token, or is a token missing from values, is copied unchanged. If final is false, a trailing
// fragment that could still become a token is left out and its offset is returned; otherwise the
// returned offset is len(src). The caller must hold the session's read lock.
func restoreAppend[T string | []byte](values map[string]string, dst []byte, src T, final bool) ([]byte, int) {
	last := 0
	for i := 0; i < len(src); {
		j := indexByte(src[i:], '<')
		if j < 0 {
			break
		}
		i += j
		n, st := parseToken(src[i:])
		switch st {
		case tokenComplete:
			if v, ok := values[string(src[i:i+n])]; ok {
				dst = append(dst, src[last:i]...)
				dst = append(dst, v...)
				last = i + n
			}
			i += n
		case tokenPartial:
			if !final {
				return append(dst, src[last:i]...), i
			}
			i = len(src) // an unfinished token at the end of the stream is not a token
		default:
			i++
		}
	}
	return append(dst, src[last:]...), len(src)
}

func indexByte[T string | []byte](s T, c byte) int {
	switch v := any(s).(type) {
	case string:
		return strings.IndexByte(v, c)
	case []byte:
		return bytes.IndexByte(v, c)
	}
	for i := range len(s) {
		if s[i] == c {
			return i
		}
	}
	return -1
}
