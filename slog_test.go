package redactbr_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/slogtest"
	"time"

	"github.com/GuilhermeMikin/redactbr"
)

// logLine logs through a handler that writes JSON, runs log on the logger, and returns the decoded record.
func logLine(t *testing.T, r *redactbr.Redactor, log func(*slog.Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	log(slog.New(r.NewSlogHandler(slog.NewJSONHandler(&buf, nil))))
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("decoding %q: %v", buf.String(), err)
	}
	return rec
}

func TestSlogHandlerConformance(t *testing.T) {
	var buf bytes.Buffer
	h := mustNew(t).NewSlogHandler(slog.NewJSONHandler(&buf, nil))
	results := func() []map[string]any {
		var out []map[string]any
		for line := range bytes.Lines(buf.Bytes()) {
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}
			out = append(out, m)
		}
		return out
	}
	if err := slogtest.TestHandler(h, results); err != nil {
		t.Error(err)
	}
}

type secretValuer struct{ cpf string }

func (v secretValuer) LogValue() slog.Value { return slog.StringValue("value " + v.cpf) }

type groupValuer struct{ cpf string }

func (v groupValuer) LogValue() slog.Value { return slog.GroupValue(slog.String("cpf", v.cpf)) }

type person struct {
	Name string
	CPF  string
}

type withPointer struct{ Doc *string }

type panicky struct{}

func (panicky) String() string { panic("broken: 123.456.789-09") }

type nilError struct{ msg string }

func (e *nilError) Error() string { return e.msg }

// doublePanic makes fmt give up: the value it panics with also panics when printed.
type doublePanic struct{}

func (doublePanic) String() string { panic(doublePanic{}) }

type namedBytes []byte

type namedByte byte

type ipLike [4]byte

func (ipLike) String() string { return "1.2.3.4" }

type bodyHolder struct{ Body string }

func TestSlogHandlerAttributes(t *testing.T) {
	const bare = "12345678909"
	secret := "123.456.789-09"
	tests := []struct {
		name string
		attr slog.Attr
		want any // the decoded JSON value of the attribute's key
	}{
		{"formatted string", slog.String("note", "customer "+secret), "customer <CPF>"},
		{"unformatted string, named", slog.String("cpf", bare), "<CPF>"},
		{"unformatted string, named in another case", slog.String("customerCPF", bare), "<CPF>"},
		{"unformatted string, not named", slog.String("id", bare), bare},
		{"keyword inside the value", slog.String("note", "CPF: "+bare), "CPF: <CPF>"},
		{"CNPJ named", slog.String("cnpj", "ab12cd34ef5602"), "<CNPJ>"},
		{"the other entity's name", slog.String("cnpj", bare), bare},
		{"email", slog.String("contact", "ana@example.com"), "<EMAIL>"},
		{"int named", slog.Int64("cpf", 12345678909), "<CPF>"},
		{"int not named", slog.Int64("id", 12345678909), float64(12345678909)},
		{"uint named", slog.Uint64("cpf", 12345678909), "<CPF>"},
		{"float named", slog.Float64("cpf", 12345678909), "<CPF>"},
		{"float not named", slog.Float64("ratio", 0.5), 0.5},
		{"CNPJ as a number", slog.Int64("cnpj", 11223344556613), "<CNPJ>"},
		{"int with invalid check digits", slog.Int64("cpf", 12345678908), float64(12345678908)},
		{"CPF that lost a leading zero", slog.Int64("cpf", 1234567890), "<CPF>"},
		{"CPF that lost a leading zero, as a uint", slog.Uint64("cpf", 1234567890), "<CPF>"},
		{"CPF that lost a leading zero, as a float", slog.Float64("cpf", 1234567890), "<CPF>"},
		{"CPF that lost three leading zeros", slog.Int64("cpf", 12345601), "<CPF>"},
		{"CNPJ that lost a leading zero", slog.Int64("cnpj", 1223344556660), "<CNPJ>"},
		{"a small counter with valid digits", slog.Int64("cpf_errors", 191), float64(191)},
		{"a lost zero without the entity's name", slog.Int64("id", 1234567890), float64(1234567890)},
		{"a lost zero in the other entity's name", slog.Int64("cnpj", 1234567890), float64(1234567890)},
		{"a long field name", slog.String("cpf_do_responsavel_legal_da_empresa_cliente", bare), "<CPF>"},
		{"bool", slog.Bool("cpf", true), true},
		{"duration", slog.Duration("cpf", time.Second), float64(time.Second)},
		{"error", slog.Any("err", errors.New("bad document "+secret)), "bad document <CPF>"},
		{"error with a named value", slog.Any("cpf", errors.New(bare)), "<CPF>"},
		{"nil pointer inside an error", slog.Any("err", (*nilError)(nil)), "<nil>"},
		{"struct", slog.Any("user", person{"Ana", bare}), "{Name:Ana CPF:<CPF>}"},
		{"pointer to struct", slog.Any("user", &person{"Ana", bare}), "&{Name:Ana CPF:<CPF>}"},
		{"slice", slog.Any("docs", []string{secret}), "[<CPF>]"},
		{"map", slog.Any("docs", map[string]string{"cpf": bare}), "map[cpf:<CPF>]"},
		{"bytes", slog.Any("raw", []byte("doc "+secret)), "doc <CPF>"},
		{"bytes without values", slog.Any("raw", []byte("ok")), "ok"},
		{"named bytes", slog.Any("raw", namedBytes("doc "+secret)), "doc <CPF>"},
		{"an array of bytes", slog.Any("raw", [14]byte([]byte(secret))), "<CPF>"},
		{"named elements", slog.Any("raw", []namedByte{'1', '2', '3', '.', '4', '5', '6', '.', '7', '8', '9', '-', '0', '9'}), "<CPF>"},
		{"an array of named elements", slog.Any("raw", [14]namedByte{'1', '2', '3', '.', '4', '5', '6', '.', '7', '8', '9', '-', '0', '9'}), "<CPF>"},
		{"a nil slice of named elements", slog.Any("raw", []namedByte(nil)), ""},
		{"a raw JSON message", slog.Any("raw", json.RawMessage(`{"email":"ana@example.com"}`)), `{"email":"<EMAIL>"}`},
		{"bytes with a String method", slog.Any("ip", ipLike{1, 2, 3, 4}), "1.2.3.4"},
		{"struct with a string", slog.Any("v", bodyHolder{secret}), "{Body:<CPF>}"},
		{"nil", slog.Any("x", nil), nil},
		{"LogValuer", slog.Any("v", secretValuer{secret}), "value <CPF>"},
		{"Stringer that panics", slog.Any("v", panicky{}), "%!v(PANIC=String method: broken: <CPF>)"},
		{"Stringer that panics with a value that panics", slog.Any("v", doublePanic{}), "!PANIC: could not format the value"},
	}
	r := mustNew(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := logLine(t, r, func(l *slog.Logger) { l.LogAttrs(context.Background(), slog.LevelInfo, "msg", tt.attr) })
			if got := rec[tt.attr.Key]; got != tt.want {
				t.Errorf("logged %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSlogHandlerDoesNotFollowPointers(t *testing.T) {
	doc := "123.456.789-09"
	var buf bytes.Buffer
	l := slog.New(mustNew(t).NewSlogHandler(slog.NewJSONHandler(&buf, nil)))
	l.Info("msg", "v", withPointer{&doc})
	if strings.Contains(buf.String(), doc) {
		t.Errorf("a value behind a pointer reached the log: %s", buf.String())
	}
}

func TestSlogHandlerMessagesKeysAndGroups(t *testing.T) {
	r := mustNew(t)
	const bare = "12345678909"

	rec := logLine(t, r, func(l *slog.Logger) { l.Info("customer 123.456.789-09 / " + bare) })
	if rec["msg"] != "customer <CPF> / "+bare {
		t.Errorf("msg = %v", rec["msg"])
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", "987.654.321-00", "x") })
	if rec["<CPF>"] != "x" {
		t.Errorf("a value in a key was not redacted: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", slog.Group("cpf", "numero", bare, "id", bare)) })
	if g := rec["cpf"].(map[string]any); g["numero"] != "<CPF>" || g["id"] != "<CPF>" {
		t.Errorf("a group name did not count as context: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", slog.Group("a", slog.Group("cliente_cpf", "b", bare))) })
	if g := rec["a"].(map[string]any)["cliente_cpf"].(map[string]any); g["b"] != "<CPF>" {
		t.Errorf("a nested group name did not count as context: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", slog.Group("user", "id", bare)) })
	if g := rec["user"].(map[string]any); g["id"] != bare {
		t.Errorf("a value was redacted without context: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", slog.Group("", "cpf", bare, "id", bare)) })
	if rec["cpf"] != "<CPF>" || rec["id"] != bare {
		t.Errorf("an inlined group: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.With("cpf", bare, "id", bare).Info("msg") })
	if rec["cpf"] != "<CPF>" || rec["id"] != bare {
		t.Errorf("With: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.WithGroup("cpf").With("a", bare).Info("msg", "b", bare) })
	if g := rec["cpf"].(map[string]any); g["a"] != "<CPF>" || g["b"] != "<CPF>" {
		t.Errorf("WithGroup: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.WithGroup("987.654.321-00").Info("msg", "a", 1) })
	if _, ok := rec["<CPF>"]; !ok {
		t.Errorf("a value in a group name was not redacted: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", "v", groupValuer{bare}) })
	if g := rec["v"].(map[string]any); g["cpf"] != "<CPF>" {
		t.Errorf("a LogValuer returning a group: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", slog.Group("cpfs", bare, "ativo")) })
	if g := rec["cpfs"].(map[string]any); g["<CPF>"] != "ativo" {
		t.Errorf("a key in a group named after an entity was not redacted: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", slog.Any("987.654.321-00", nil)) })
	if v, ok := rec["<CPF>"]; !ok || v != nil {
		t.Errorf("a key with a nil value was not redacted: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.WithGroup("cpf").WithGroup(bare).Info("msg", "a", 1) })
	if g := rec["cpf"].(map[string]any); g["<CPF>"] == nil {
		t.Errorf("a group name in a group named after an entity was not redacted: %v", rec)
	}

	rec = logLine(t, r, func(l *slog.Logger) { l.Info("msg", "tipo", "cpf", "doc", bare) })
	if rec["doc"] != bare {
		t.Errorf("another attribute counted as context: %v", rec)
	}
}

func TestSlogHandlerPolicyAndZeroRedactor(t *testing.T) {
	strict := mustNew(t, redactbr.WithPolicy(redactbr.Strict))
	rec := logLine(t, strict, func(l *slog.Logger) { l.Info("msg", "id", "12345678909", "n", 12345678909) })
	if rec["id"] != "<CPF>" || rec["n"] != "<CPF>" {
		t.Errorf("Strict: %v", rec)
	}
	var zero redactbr.Redactor
	rec = logLine(t, &zero, func(l *slog.Logger) { l.Info("123.456.789-09") })
	if rec["msg"] != "<CPF>" {
		t.Errorf("zero Redactor: %v", rec)
	}
	only := mustNew(t, redactbr.WithEntities(redactbr.Email))
	rec = logLine(t, only, func(l *slog.Logger) { l.Info("msg", "cpf", "123.456.789-09", "e", "ana@example.com") })
	if rec["cpf"] != "123.456.789-09" || rec["e"] != "<EMAIL>" {
		t.Errorf("WithEntities: %v", rec)
	}
}

// recordingHandler keeps what it gets, to look at it without an encoder in between.
type recordingHandler struct {
	mu      *sync.Mutex
	records *[]slog.Record
	err     error
	level   slog.Level
}

func newRecordingHandler() recordingHandler {
	return recordingHandler{mu: &sync.Mutex{}, records: &[]slog.Record{}}
}

func (h recordingHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h recordingHandler) Handle(_ context.Context, rec slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = append(*h.records, rec)
	return h.err
}

func (h recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestSlogHandlerPassesThrough(t *testing.T) {
	r := mustNew(t)
	next := newRecordingHandler()
	next.level = slog.LevelWarn
	h := r.NewSlogHandler(next)

	if h.Enabled(context.Background(), slog.LevelInfo) || !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled does not follow the next handler")
	}
	if h.WithAttrs(nil) != h || h.WithGroup("") != h {
		t.Error("WithAttrs(nil) and WithGroup(\"\") must return the same handler")
	}

	// The time, level and source survive, and the original record is left as it was.
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	rec := slog.NewRecord(at, slog.LevelWarn, "CPF 123.456.789-09", 1234)
	rec.AddAttrs(slog.String("cpf", "12345678909"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	got := (*next.records)[0]
	if !got.Time.Equal(at) || got.Level != slog.LevelWarn || got.PC != 1234 || got.Message != "CPF <CPF>" {
		t.Errorf("unexpected record: %+v", got)
	}
	rec.Attrs(func(a slog.Attr) bool {
		if a.Value.String() != "12345678909" {
			t.Error("the original record was modified")
		}
		return true
	})
	if rec.Message != "CPF 123.456.789-09" {
		t.Error("the original record was modified")
	}

	// An error from the next handler comes back as it is.
	boom := errors.New("disk full")
	failing := newRecordingHandler()
	failing.err = boom
	if err := r.NewSlogHandler(failing).Handle(context.Background(), rec); !errors.Is(err, boom) {
		t.Errorf("Handle returned %v", err)
	}
}

// Deep group nesting comes from code, not input, but it should not cost more than the depth.
func TestSlogHandlerKeepsNoGrowingContext(t *testing.T) {
	h := mustNew(t).NewSlogHandler(newRecordingHandler())
	for range 20000 {
		h = h.WithGroup("group")
	}
	rec := slog.NewRecord(time.Time{}, slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("cpf", "12345678909"))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func TestSlogHandlerConcurrentUse(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(mustNew(t).NewSlogHandler(slog.NewJSONHandler(&buf, nil)))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				l.With("cpf", "12345678909").WithGroup("g").Info("CPF 123.456.789-09", "cpf", "98765432100")
			}
		})
	}
	wg.Wait()
	for _, value := range []string{"12345678909", "98765432100", "123.456.789-09"} {
		if strings.Contains(buf.String(), value) {
			t.Fatalf("a value reached the log")
		}
	}
}

// FuzzSlogHandler checks that nothing detectable is left, under Strict, in a message, a key or a value.
func FuzzSlogHandler(f *testing.F) {
	f.Add("CPF 123.456.789-09", "cpf", "12345678909")
	f.Add("msg", "123.456.789-09", "x")
	f.Add("a@b.co", "e=", "a@b.co")
	f.Add("", "", "")
	f.Add("x", "12345678909", "98765432100")
	f.Add("x", "cnpj", "ab12cd34/ef56-02")
	r := mustNew(f, redactbr.WithPolicy(redactbr.Strict))
	f.Fuzz(func(t *testing.T, msg, key, value string) {
		next := newRecordingHandler()
		rec := slog.NewRecord(time.Time{}, slog.LevelInfo, msg, 0)
		rec.AddAttrs(slog.String(key, value), slog.Any(key, value+key), slog.Group(key, slog.String(key, value)))
		if err := r.NewSlogHandler(next).Handle(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
		got := (*next.records)[0]
		check := func(s string) {
			if left := r.Find(s); len(left) > 0 {
				t.Fatalf("detectable values are left: %v", left)
			}
		}
		check(got.Message)
		var walk func(a slog.Attr)
		walk = func(a slog.Attr) {
			check(a.Key)
			if a.Value.Kind() == slog.KindGroup {
				for _, ga := range a.Value.Group() {
					walk(ga)
				}
				return
			}
			check(a.Value.String())
		}
		got.Attrs(func(a slog.Attr) bool {
			walk(a)
			return true
		})
	})
}

func BenchmarkSlogHandler(b *testing.B) {
	l := slog.New(mustNew(b).NewSlogHandler(slog.NewJSONHandler(io.Discard, nil)))
	b.ReportAllocs()
	for b.Loop() {
		l.Info("customer 123.456.789-09 signed up", "cpf", "12345678909", "email", "ana@example.com", "plan", "pro", "n", 42)
	}
}

func BenchmarkSlogBaseline(b *testing.B) {
	l := slog.New(slog.NewJSONHandler(io.Discard, nil))
	b.ReportAllocs()
	for b.Loop() {
		l.Info("customer 123.456.789-09 signed up", "cpf", "12345678909", "email", "ana@example.com", "plan", "pro", "n", 42)
	}
}

func ExampleRedactor_NewSlogHandler() {
	r, err := redactbr.New()
	if err != nil {
		fmt.Println(err)
		return
	}
	// The time is removed only to keep this example's output stable.
	opts := &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}
	logger := slog.New(r.NewSlogHandler(slog.NewJSONHandler(printer{}, opts)))
	logger.Info("signup from ana@example.com", "cpf", "12345678909", "order", "98765432100", "err", errors.New("CPF 123.456.789-09 is locked"))
	// Output:
	// {"level":"INFO","msg":"signup from <EMAIL>","cpf":"<CPF>","order":"98765432100","err":"CPF <CPF> is locked"}
}

type printer struct{}

func (printer) Write(p []byte) (int, error) { return fmt.Print(string(p)) }
