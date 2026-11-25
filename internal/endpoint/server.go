package endpoint

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type Options struct {
	Listen         string
	Certificate    tls.Certificate
	AllowedOrigins []string
	JWTKey         []byte
	MCP            http.Handler
	Now            func() time.Time
	Logger         *slog.Logger
}

type Server struct {
	opts Options
}

func New(opts Options) (*Server, error) {
	return nil, errors.ErrUnsupported
}

func (s *Server) Handler() http.Handler {
	return nil
}

func (s *Server) Run(ctx context.Context) error {
	return errors.ErrUnsupported
}
