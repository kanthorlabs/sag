package agentsetup

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kanthorlabs/sag/internal/prompt"
	"github.com/kanthorlabs/sag/internal/secret"
	"golang.org/x/term"
)

var remoteOpenTTY = func() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}

func ParseURL(rawURL string) (Mode, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Port() != "8443" || u.Path != "/mcp" {
		return ModeLocal, fmt.Errorf("--url must be https://<host>:8443/mcp")
	}
	return ModeRemote, nil
}

func ReadJWT(tty io.ReadWriter, sub string) (secret.Credential, error) {
	fmt.Fprintf(tty, "JWT for %s (input hidden): ", sub)
	var raw []byte
	if f, ok := tty.(*os.File); ok {
		b, err := term.ReadPassword(int(f.Fd()))
		if err == nil {
			raw = b
			fmt.Fprintln(tty)
		} else {
			b2, err2 := remoteReadLine(tty)
			if err2 != nil {
				return secret.Credential{}, err2
			}
			raw = b2
		}
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

func planRemote(ctx context.Context, opts Options) error {
	caPath := opts.CfgDir + "/ca.pem"
	if _, err := os.Stat(caPath); err != nil {
		return fmt.Errorf("fail ca.pem absent for remote mode")
	}

	mcpFile := remoteMCPFile(opts)
	if conflict := remoteCheckVariableHeader(mcpFile); conflict != "" {
		return fmt.Errorf("%s", conflict)
	}

	if remoteNeedsJWT(mcpFile, opts.URL, opts.Sub) {
		tty, err := remoteOpenTTY()
		if err != nil {
			return fmt.Errorf("open /dev/tty: %w", prompt.ErrNoTerminal)
		}
		tty.Close()
	}
	return nil
}

func remoteJWT(ctx context.Context, opts Options) (secret.Credential, error) {
	tty, err := remoteOpenTTY()
	if err != nil {
		return secret.Credential{}, fmt.Errorf("open /dev/tty: %w", prompt.ErrNoTerminal)
	}
	defer tty.Close()
	return ReadJWT(tty, opts.Sub)
}

func remoteMCPFile(opts Options) string {
	switch opts.Harness {
	case HarnessPi:
		return PiPaths(opts.Env["PI_CODING_AGENT_DIR"], opts.Home).MCPFile
	default:
		return ClaudeCodePaths(opts.Env["CLAUDE_CONFIG_DIR"], opts.Home).MCPFile
	}
}

func remoteCheckVariableHeader(mcpFile string) string {
	_, auth := remoteReadEntry(mcpFile)
	if auth == "${SAG_JWT}" {
		return "fail mcp-server sag in " + mcpFile + " has another form; remove it"
	}
	return ""
}

func remoteNeedsJWT(mcpFile, wantURL, wantSub string) bool {
	entryURL, auth := remoteReadEntry(mcpFile)
	if entryURL != wantURL {
		return true
	}
	if !strings.HasPrefix(auth, "Bearer ") {
		return true
	}
	raw := strings.TrimPrefix(auth, "Bearer ")
	return remoteSubFromJWT(raw) != wantSub
}

func remoteReadEntry(mcpFile string) (entryURL, authHeader string) {
	data, err := os.ReadFile(mcpFile)
	if err != nil {
		return "", ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return "", ""
	}
	serversRaw, ok := top["mcpServers"]
	if !ok {
		return "", ""
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		return "", ""
	}
	entryRaw, ok := servers["sag"]
	if !ok {
		return "", ""
	}
	var entry struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		return "", ""
	}
	return entry.URL, entry.Headers["Authorization"]
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
