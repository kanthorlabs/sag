package scrub

import (
	"cmp"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/kanthorlabs/sag/internal/core"
	"github.com/kanthorlabs/sag/internal/secret"
)

const redacted = "[REDACTED]"

const passwordKey = "password"

type scrubState struct {
	mu   sync.RWMutex
	set  map[string]bool
	repl *strings.Replacer
}

type Scrubber struct {
	state *scrubState
}

func New(secrets []secret.Credential) Scrubber {
	st := &scrubState{set: map[string]bool{}}
	for _, c := range secrets {
		addForms(st.set, c)
	}
	if len(st.set) > 0 {
		st.repl = buildReplacer(st.set)
	}
	return Scrubber{state: st}
}

func addForms(set map[string]bool, c secret.Credential) {
	value := string(c.Reveal())
	trimmed := strings.TrimSpace(value)
	values := append([]string{value, trimmed}, urlPasswords(trimmed)...)
	values = append(values, kvPasswords(trimmed)...)
	for _, v := range values {
		for _, form := range encodedForms(v) {
			if form != "" {
				set[form] = true
			}
		}
	}
}

func buildReplacer(set map[string]bool) *strings.Replacer {
	needles := make([]string, 0, len(set))
	for n := range set {
		needles = append(needles, n)
	}
	slices.SortFunc(needles, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b))
	})
	pairs := make([]string, 0, 2*len(needles))
	for _, n := range needles {
		pairs = append(pairs, n, redacted)
	}
	return strings.NewReplacer(pairs...)
}

func (s Scrubber) Add(c secret.Credential) {
	if s.state == nil {
		return
	}
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	before := len(s.state.set)
	addForms(s.state.set, c)
	if len(s.state.set) > before {
		s.state.repl = buildReplacer(s.state.set)
	}
}

func (s Scrubber) String(text string) string {
	if s.state == nil {
		return text
	}
	s.state.mu.RLock()
	repl := s.state.repl
	s.state.mu.RUnlock()
	if repl == nil {
		return text
	}
	return repl.Replace(text)
}

func (s Scrubber) Error(err error) error {
	if err == nil {
		return nil
	}
	var ce *core.Error
	if !errors.As(err, &ce) {
		return errors.New(s.String(err.Error()))
	}
	out := &core.Error{Code: ce.Code, Message: ce.Message, Trace: ce.Trace}
	if ce.Cause != nil {
		out.Cause = errors.New(s.String(chainText(ce.Cause)))
	}
	return out
}

func chainText(err error) string {
	parts := []string{}
	for err != nil {
		ce, ok := err.(*core.Error)
		if !ok {
			parts = append(parts, err.Error())
			break
		}
		parts = append(parts, ce.Error())
		err = ce.Cause
	}
	return strings.Join(parts, ": ")
}

func encodedForms(v string) []string {
	return []string{
		v,
		url.QueryEscape(v),
		url.PathEscape(v),
		strings.TrimSuffix(strings.TrimPrefix(strconv.Quote(v), `"`), `"`),
		strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v),
	}
}

func urlPasswords(v string) []string {
	u, err := url.Parse(v)
	if err != nil {
		return nil
	}
	out := u.Query()[passwordKey]
	if u.User == nil {
		return out
	}
	if p, ok := u.User.Password(); ok {
		out = append(out, p)
	}
	if _, rest, ok := strings.Cut(v, "://"); ok {
		authority, _, _ := strings.Cut(rest, "/")
		if i := strings.LastIndex(authority, "@"); i >= 0 {
			if _, raw, ok := strings.Cut(authority[:i], ":"); ok {
				out = append(out, raw)
			}
		}
	}
	return out
}

func kvPasswords(v string) []string {
	var out []string
	rest := v
	for {
		i := strings.Index(rest, passwordKey)
		if i < 0 {
			return out
		}
		rest = rest[i+len(passwordKey):]
		after := strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(after, "=") {
			continue
		}
		after = strings.TrimLeft(after[1:], " \t")
		value, n := kvValue(after)
		out = append(out, value)
		rest = after[n:]
	}
}

func kvValue(s string) (string, int) {
	if !strings.HasPrefix(s, "'") {
		end := strings.IndexAny(s, " \t\r\n")
		if end < 0 {
			end = len(s)
		}
		return s[:end], end
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				b.WriteByte(s[i])
			}
		case '\'':
			return b.String(), i + 1
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String(), len(s)
}
