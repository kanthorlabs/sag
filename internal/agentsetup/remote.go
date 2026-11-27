package agentsetup

import (
	"bufio"
	"context"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	"golang.org/x/term"
	"io"
	"net/url"
	"os"
)

var remoteOpenTTY = func() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func ParseURL(rawURL string) (Mode, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Port() != "8443" || u.Path != "/mcp" {
		return ModeLocal, fmt.Errorf("--url must be https://<host>:8443/mcp")
	}
	if u.User != nil {
		return ModeLocal, fmt.Errorf("--url must be https://<host>:8443/mcp")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return ModeLocal, fmt.Errorf("--url must be https://<host>:8443/mcp")
	}
	return ModeRemote, nil
}

func ReadJWT(tty io.ReadWriter, sub string) (secret.Credential, error) {
	fmt.Fprintf(tty, "JWT for %s (input hidden): ", sub)
	var raw []byte
	if f, ok := tty.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		if err != nil {
			return secret.Credential{}, fmt.Errorf("read password: %w", err)
		}
		fmt.Fprintln(tty)
		raw = b
	} else {
		b, err := remoteReadLine(tty)
		if err != nil {
			return secret.Credential{}, err
		}
		raw = b
	}
	jwtSub := remoteSubFromJWT(string(raw))
	if jwtSub != sub {
		return secret.Credential{}, fmt.Errorf("jwt sub %q does not match --sub %q", jwtSub, sub)
	}
	return secret.NewCredential("jwt-"+sub, raw), nil
}

func remoteJWT(ctx context.Context, opts Options) (secret.Credential, error) {
	tty, err := remoteOpenTTY()
	if err != nil {
		return secret.Credential{}, fmt.Errorf("open /dev/tty: %w", prompt.ErrNoTerminal)
	}
	defer tty.Close()
	return ReadJWT(tty, opts.Sub)
}

func remoteSubFromJWT(raw string) string {
	p := jwt.NewParser(jwt.WithoutClaimsValidation(), jwt.WithJSONNumber())
	var claims jwt.MapClaims
	_, _, err := p.ParseUnverified(raw, &claims)
	if err != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func remoteReadLine(r io.Reader) ([]byte, error) {
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return nil, err
		}
		return []byte{}, nil
	}
	return sc.Bytes(), nil
}
