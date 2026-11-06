package redactbr

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"

	"github.com/GuilhermeMikin/redactbr/internal/cnpj"
	"github.com/GuilhermeMikin/redactbr/internal/cpf"
)

// maxLostZeros is how many leading zeros a number in a field named after an entity may have lost. Allowing more
// would turn small counters ("cpf_errors": 191) into false positives.
const maxLostZeros = 3

// NewSlogHandler returns a slog.Handler that replaces detected values in each record with their entity label, as
// Redact does, and passes the result to next. The name of a field and of the groups around it are context for its
// value under the Balanced policy, so slog.String("cpf", "12345678909") is redacted and slog.String("id",
// "12345678909") is not. Only those names count: neither the message nor other attributes are context for it.
//
// Strings and numbers are checked, including, in a field named after an entity, a number that lost its leading
// zeros. Booleans, durations and times pass through. Byte slices and arrays are read as text. Any other value, such
// as an error or a struct, is logged as a string: its redacted %+v text (%v for errors). The handler cannot see
// inside an arbitrary value, and a JSON handler would otherwise marshal fields that the text does not show.
// Structured values therefore lose their structure in the log.
func (r *Redactor) NewSlogHandler(next slog.Handler) slog.Handler {
	return &slogHandler{r: r, next: next}
}

type slogHandler struct {
	r      *Redactor
	next   slog.Handler
	groups fieldContext // what the names of the open groups say
}

// A fieldContext says which keywords occur in the names around a value. A name counts whole, whatever its length:
// it is a label, not nearby text.
type fieldContext struct{ cpf, cnpj bool }

func (c fieldContext) with(name string) fieldContext {
	return fieldContext{
		cpf:  c.cpf || containsKeyword(name, cpfKeyword),
		cnpj: c.cnpj || containsKeyword(name, cnpjKeyword),
	}
}

func (h *slogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *slogHandler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.r.Redact(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.attr(h.groups, a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.attr(h.groups, a)
	}
	return &slogHandler{h.r, h.next.WithAttrs(redacted), h.groups}
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &slogHandler{h.r, h.next.WithGroup(h.r.redactInContext(h.groups, name)), h.groups.with(name)}
}

// attr returns a redacted. The groups around it are context for its key, and those and the key for its value.
func (h *slogHandler) attr(groups fieldContext, a slog.Attr) slog.Attr {
	key := h.r.redactInContext(groups, a.Key)
	c := groups.with(a.Key)
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindGroup:
		attrs := v.Group()
		out := make([]slog.Attr, len(attrs))
		for i, ga := range attrs {
			out[i] = h.attr(c, ga)
		}
		v = slog.GroupValue(out...)
	case slog.KindString:
		v = slog.StringValue(h.r.redactInContext(c, v.String()))
	case slog.KindInt64:
		v = h.number(c, v, strconv.FormatInt(v.Int64(), 10))
	case slog.KindUint64:
		v = h.number(c, v, strconv.FormatUint(v.Uint64(), 10))
	case slog.KindFloat64:
		v = h.number(c, v, strconv.FormatFloat(v.Float64(), 'f', -1, 64))
	case slog.KindAny:
		if v.Any() != nil {
			v = slog.StringValue(h.r.redactInContext(c, textOf(v.Any())))
		}
	}
	return slog.Attr{Key: key, Value: v}
}

// number keeps v unless its digits, as text, contain something to redact. A number has no leading zeros, so in a
// field named after an entity it is also tried with the zeros it may have lost: 1234567890 for 012.345.678-90.
func (h *slogHandler) number(c fieldContext, v slog.Value, digits string) slog.Value {
	if redacted := h.r.redactInContext(c, digits); redacted != digits {
		return slog.StringValue(redacted)
	}
	for _, e := range [...]struct {
		named  bool
		length int
	}{{c.cpf, cpf.Len}, {c.cnpj, cnpj.Len}} {
		if lost := e.length - len(digits); e.named && lost >= 1 && lost <= maxLostZeros {
			padded := strings.Repeat("0", lost) + digits
			if redacted := h.r.redactInContext(c, padded); redacted != padded {
				return slog.StringValue(redacted)
			}
		}
	}
	return v
}

// textOf returns the text of an arbitrary value. fmt reports a panic in a value's own method in the text, which
// is then redacted. A panic that fmt cannot survive becomes a fixed text, which holds nothing of the value.
func textOf(x any) (text string) {
	defer func() {
		if recover() != nil {
			text = "!PANIC: could not format the value"
		}
	}()
	switch x := x.(type) {
	case []byte:
		return string(x)
	case error:
		return fmt.Sprint(x) // not x.Error(): fmt survives a nil pointer inside an error
	case fmt.Stringer, fmt.Formatter:
		return fmt.Sprintf("%+v", x)
	}
	// fmt would print the other kinds of bytes as a list of numbers, which nothing could detect anything in.
	rv := reflect.ValueOf(x)
	if k := rv.Kind(); (k == reflect.Slice || k == reflect.Array) && rv.Type().Elem().Kind() == reflect.Uint8 {
		b := make([]byte, rv.Len())
		reflect.Copy(reflect.ValueOf(b), rv)
		return string(b)
	}
	return fmt.Sprintf("%+v", x)
}

// redactInContext redacts text as Redact does, as if the keywords in c came right before it.
func (r *Redactor) redactInContext(c fieldContext, text string) string {
	var prefix string
	switch {
	case c.cpf && c.cnpj:
		prefix = cpfKeyword + "/" + cnpjKeyword + "="
	case c.cpf:
		prefix = cpfKeyword + "="
	case c.cnpj:
		prefix = cnpjKeyword + "="
	default:
		return r.Redact(text)
	}
	labeled := prefix + text
	ms := r.find(labeled)
	if len(ms) == 0 {
		return text
	}
	return replaceMatches(labeled, ms)[len(prefix):]
}
