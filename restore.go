package redactbr

import (
	"errors"
	"fmt"
	"io"
)

var errWriterClosed = errors.New("redactbr: write to a closed RestoreWriter")

// RestoreWriter restores a session's tokens in a stream, such as an LLM response arriving in chunks, and
// writes the result to an underlying writer. A token split across writes is restored as long as it is
// complete before Close; to make that possible, at most 17 bytes (one token minus one byte) that could be
// the start of a token are held back between writes. Everything else is written through unchanged.
//
// Call Close at the end of the stream to write the held-back bytes; an unfinished token is written as it
// is. Close does not close the underlying writer. A RestoreWriter is not safe for concurrent use; use one
// per stream.
type RestoreWriter struct {
	s      *Session
	w      io.Writer
	buf    []byte // held-back bytes at buf[:held], then the current write
	held   int
	out    []byte
	err    error
	closed bool
}

// RestoreWriter returns a RestoreWriter that writes to w. Tokens the session issues after this call are
// restored too.
func (s *Session) RestoreWriter(w io.Writer) *RestoreWriter {
	return &RestoreWriter{s: s, w: w}
}

// Write restores the tokens in p and writes the result, holding back a possible partial token at the end.
// It returns len(p) on success. After the underlying writer fails, Write returns that error for every
// call.
func (rw *RestoreWriter) Write(p []byte) (int, error) {
	if rw.err != nil {
		return 0, rw.err
	}
	if rw.closed {
		return 0, errWriterClosed
	}
	rw.buf = append(rw.buf[:rw.held], p...)
	rw.s.mu.RLock()
	out, keep := restoreAppend(rw.s.values, rw.out[:0], rw.buf, false)
	rw.s.mu.RUnlock()
	rw.out = out
	rw.held = copy(rw.buf, rw.buf[keep:])
	if err := rw.flush(); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close writes the held-back bytes. Further writes fail; further calls to Close return nil.
func (rw *RestoreWriter) Close() error {
	if rw.closed {
		return nil
	}
	rw.closed = true
	if rw.err != nil {
		return rw.err
	}
	rw.s.mu.RLock()
	out, _ := restoreAppend(rw.s.values, rw.out[:0], rw.buf[:rw.held], true)
	rw.s.mu.RUnlock()
	rw.out = out
	rw.held = 0
	return rw.flush()
}

func (rw *RestoreWriter) flush() error {
	if len(rw.out) == 0 {
		return nil
	}
	if _, err := rw.w.Write(rw.out); err != nil {
		rw.err = err
		return err
	}
	return nil
}

// String describes the writer without revealing buffered text, which may hold restored values.
func (rw *RestoreWriter) String() string {
	return fmt.Sprintf("redactbr.RestoreWriter{held: %d, closed: %t}", rw.held, rw.closed)
}

// GoString is like String.
func (rw *RestoreWriter) GoString() string {
	return rw.String()
}
