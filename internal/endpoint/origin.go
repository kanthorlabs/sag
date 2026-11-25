package endpoint

import "net/http"

func (s *Server) originCheck(next http.Handler) http.Handler {
	return next
}
