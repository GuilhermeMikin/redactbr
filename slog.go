package redactbr

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

// NewSlogHandler returns a slog.Handler that replaces detected values in each record with their entity label, as
// Redact does, and passes the result to next. The name of a field is context for its value under the Balanced
// policy, so slog.String("cpf", "12345678909") is redacted and slog.String("id", "12345678909") is not. Only
// the field's own name counts: neither the message nor other attributes are context for it.
//
// Strings and numbers are checked. Booleans, durations and times pass through. Any other value, such as an
// error or a struct, is logged as a string: its redacted %+v text. The handler cannot see inside an arbitrary
// value, and a JSON handler would otherwise marshal fields that the text does not show. Structured values
// therefore lose their structure in the log.
func (r *Redactor) NewSlogHandler(next slog.Handler) slog.Handler {
	return &slogHandler{r: r, next: next}
}

type slogHandler struct {
	r      *Redactor
	next   slog.Handler
	groups string // names of the open groups, each followed by a '.', as context for attributes
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
	return &slogHandler{h.r, h.next.WithGroup(h.r.Redact(name)), h.groups + name + "."}
}

// attr returns a redacted with its value judged in the context of the groups that hold it.
func (h *slogHandler) attr(groups string, a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	name := groups + a.Key
	switch v.Kind() {
	case slog.KindGroup:
		prefix := groups
		if a.Key != "" { // a group without a key is inlined by handlers
			prefix = name + "."
		}
		attrs := v.Group()
		out := make([]slog.Attr, len(attrs))
		for i, ga := range attrs {
			out[i] = h.attr(prefix, ga)
		}
		v = slog.GroupValue(out...)
	case slog.KindString:
		v = slog.StringValue(h.r.redactField(name, v.String()))
	case slog.KindInt64:
		v = h.number(name, v, strconv.FormatInt(v.Int64(), 10))
	case slog.KindUint64:
		v = h.number(name, v, strconv.FormatUint(v.Uint64(), 10))
	case slog.KindFloat64:
		v = h.number(name, v, strconv.FormatFloat(v.Float64(), 'f', -1, 64))
	case slog.KindAny:
		var text string
		switch x := v.Any().(type) {
		case nil:
			return slog.Attr{Key: h.r.Redact(a.Key)}
		case []byte:
			text = string(x)
		case error:
			text = fmt.Sprint(x) // not x.Error(): fmt survives a nil pointer inside an error
		default:
			text = fmt.Sprintf("%+v", x)
		}
		v = slog.StringValue(h.r.redactField(name, text))
	}
	return slog.Attr{Key: h.r.Redact(a.Key), Value: v}
}

// number keeps v unless its digits, as text, contain something to redact.
func (h *slogHandler) number(name string, v slog.Value, digits string) slog.Value {
	if redacted := h.r.redactField(name, digits); redacted != digits {
		return slog.StringValue(redacted)
	}
	return v
}

// redactField redacts value as the value of a field called name, which counts as context: it is judged as if it
// followed "name=". The '=' is a boundary for every entity, so no match spans both.
func (r *Redactor) redactField(name, value string) string {
	prefix := name + "="
	if !keywordBefore(prefix, len(prefix), cpfKeyword) && !keywordBefore(prefix, len(prefix), cnpjKeyword) {
		return r.Redact(value)
	}
	text := prefix + value
	var inValue []Match
	for _, m := range r.find(text) {
		if m.Start >= len(prefix) { // a value in the name itself is redacted with the name
			inValue = append(inValue, m)
		}
	}
	if len(inValue) == 0 {
		return value
	}
	return replaceMatches(text, inValue)[len(prefix):]
}
