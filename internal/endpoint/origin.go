package endpoint

import (
	"net/http"
	"slices"
)

const causeOrigin = "origin"

func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Origin")
		if len(values) == 0 || len(values) == 1 && slices.Contains(s.opts.AllowedOrigins, values[0]) {
			next.ServeHTTP(w, r)
			return
		}
		if s.opts.Audit != nil {
			s.opts.Audit(r.Context(), r.RemoteAddr, causeOrigin)
		}
		w.WriteHeader(http.StatusForbidden)
	})
}
