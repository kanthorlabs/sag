package endpoint

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	maxBodyBytes      = 64 << 10
	maxHeaderBytes    = 16 << 10
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	idleTimeout       = 60 * time.Second
	healthzPath       = "/healthz"
	shutdownTimeout   = 10 * time.Second
)

var (
	errNoMCPHandler  = errors.New("endpoint: MCP handler is required")
	errNoCertificate = errors.New("endpoint: TLS certificate is required")
	errNoJWTKey      = errors.New("endpoint: JWT key is required")
)

type Options struct {
	Listen         string
	Certificate    tls.Certificate
	AllowedOrigins []string
	JWTKey         []byte
	MCP            http.Handler
	Now            func() time.Time
	Logger         *slog.Logger
	Audit          AuditHook
}

type Server struct {
	opts Options
}

func New(opts Options) (*Server, error) {
	if opts.MCP == nil {
		return nil, errNoMCPHandler
	}
	if len(opts.Certificate.Certificate) == 0 {
		return nil, errNoCertificate
	}
	if len(opts.JWTKey) == 0 {
		return nil, errNoJWTKey
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{opts: opts}, nil
}

func (s *Server) Handler() http.Handler {
	protected := http.NewServeMux()
	protected.Handle("/mcp", bridgeTokenInfo(s.opts.MCP))
	authenticated := s.authenticate(protected)

	return s.originCheck(limitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == healthzPath && r.URL.RawPath == "" {
			healthz(w, r)
			return
		}
		authenticated.ServeHTTP(w, r)
	})))
}

func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.opts.Listen)
	if err != nil {
		return err
	}
	return s.serve(ctx, ln)
}

func (s *Server) serve(ctx context.Context, ln net.Listener) error {
	srv := s.httpServer()
	served := make(chan error, 1)
	go func() { served <- srv.ServeTLS(ln, "", "") }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	err := srv.Shutdown(shutdownCtx)
	if serveErr := <-served; !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return err
}

func (s *Server) httpServer() *http.Server {
	return &http.Server{
		Handler: s.Handler(),
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{s.opts.Certificate},
		},
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(s.opts.Logger.Handler(), slog.LevelWarn),
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBodyBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}
