package endpoint

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/kanthorlabs/sag/internal/audit"
	"github.com/kanthorlabs/sag/internal/token"
)

const (
	bearerPrefixLength = len("Bearer ")
	causeMissingBearer = "missing_bearer"
)

var bearerHeader = regexp.MustCompile(`^(?i)Bearer [^\s,]+$`)

type claimsKey struct{}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 || !bearerHeader.MatchString(values[0]) {
			s.refuse(w, r, causeMissingBearer)
			return
		}
		claims, err := token.Verify(s.opts.JWTKey, values[0][bearerPrefixLength:], s.opts.Now())
		if err != nil {
			s.refuse(w, r, verifyCause(err))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
	})
}

func (s *Server) refuse(w http.ResponseWriter, r *http.Request, cause string) {
	if s.opts.Audit != nil {
		s.opts.Audit(r.Context(), r.RemoteAddr, cause)
	}
	unauthorized(w)
}

func verifyCause(err error) string {
	var tokenErr *token.Error
	if errors.As(err, &tokenErr) {
		return string(tokenErr.Cause)
	}
	return string(token.CauseUnauthenticated)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
}

func bridgeTokenInfo(next http.Handler) http.Handler {
	return auth.RequireBearerToken(verifiedClaims, nil)(next)
}

func verifiedClaims(ctx context.Context, _ string, r *http.Request) (*auth.TokenInfo, error) {
	claims, ok := ctx.Value(claimsKey{}).(token.Claims)
	if !ok {
		return nil, auth.ErrInvalidToken
	}
	extra := map[string]any{audit.ExtraJTI: claims.JTI, audit.ExtraRemoteAddr: r.RemoteAddr}
	return &auth.TokenInfo{UserID: claims.Sub, Expiration: claims.ExpiresAt, Extra: extra}, nil
}
