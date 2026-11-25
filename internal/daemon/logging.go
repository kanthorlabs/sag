package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kanthorlabs/sag/internal/scrub"
)

type scrubbingHandler struct {
	next     slog.Handler
	scrubber scrub.Scrubber
}

func scrubHandler(h slog.Handler, s scrub.Scrubber) slog.Handler {
	return scrubbingHandler{next: h, scrubber: s}
}

func (h scrubbingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h scrubbingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, h.scrubber.String(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.attr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h scrubbingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = h.attr(a)
	}
	return scrubbingHandler{next: h.next.WithAttrs(clean), scrubber: h.scrubber}
}

func (h scrubbingHandler) WithGroup(name string) slog.Handler {
	return scrubbingHandler{next: h.next.WithGroup(h.scrubber.String(name)), scrubber: h.scrubber}
}

func (h scrubbingHandler) attr(a slog.Attr) slog.Attr {
	a.Value = h.value(a.Value.Resolve())
	return a
}

func (h scrubbingHandler) value(v slog.Value) slog.Value {
	switch v.Kind() {
	case slog.KindString:
		return slog.StringValue(h.scrubber.String(v.String()))
	case slog.KindGroup:
		group := v.Group()
		clean := make([]slog.Attr, len(group))
		for i, a := range group {
			clean[i] = h.attr(a)
		}
		return slog.GroupValue(clean...)
	case slog.KindAny:
		text := fmt.Sprint(v.Any())
		clean := h.scrubber.String(text)
		encoded, _ := json.Marshal(v.Any())
		if clean != text || h.scrubber.String(string(encoded)) != string(encoded) {
			return slog.StringValue(clean)
		}
		return v
	default:
		return v
	}
}
