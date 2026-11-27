package agentsetup

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/kanthorlabs/sag/internal/stepline"
)

func applyCATrust(ctx context.Context, opts Options) error {
	out := stepline.New(agentOut)
	ca := caFilePath(opts)

	switch opts.Harness {
	case HarnessPi:
		return applyCATrustPi(opts, ca, out)
	default:
		return applyCATrustClaudeCode(opts, ca, out)
	}
}

func applyCATrustClaudeCode(opts Options, caPath string, out stepline.Writer) error {
	paths := ClaudeCodePaths(opts.Env["CLAUDE_CONFIG_DIR"], opts.Home)
	current := currentNECCClaudeCode(opts, paths.SettingsFile)
	if current != "" {
		if acceptsCATrust(current, caPath) {
			out.Line(stepline.StatusOK, "ca-trust", "")
			return nil
		}
		return bundleConflictError(current, caPath, paths.SettingsFile)
	}
	if err := writeJSON(paths.SettingsFile, []string{"env", "NODE_EXTRA_CA_CERTS"}, caPath); err != nil {
		return err
	}
	out.Line(stepline.StatusOK, "ca-trust", "")
	return nil
}

func applyCATrustPi(opts Options, caPath string, out stepline.Writer) error {
	shell := opts.Env["SHELL"]
	zdotdir := opts.Env["ZDOTDIR"]
	profile, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
	if !ok {
		return fmt.Errorf("ca-trust: unsupported shell %q", shell)
	}

	current := currentNECCPi(opts, profile)
	if current != "" {
		if acceptsCATrust(current, caPath) {
			out.Line(stepline.StatusOK, "shell-profile", "")
			return nil
		}
		return bundleConflictError(current, caPath, profile)
	}

	if opts.ShellProfile != "yes" {
		return nil
	}

	data, _ := os.ReadFile(profile)
	var sb strings.Builder
	sb.Write(data)
	if len(data) > 0 && !bytes.HasSuffix(data, []byte("\n")) {
		sb.WriteByte('\n')
	}
	sb.WriteString("# sag agent setup\n")
	sb.WriteString(`export NODE_EXTRA_CA_CERTS="` + caPath + `"` + "\n")
	if err := os.WriteFile(profile, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	out.Line(stepline.StatusOK, "shell-profile", "")
	return nil
}

func currentNECCClaudeCode(opts Options, settingsFile string) string {
	if v := opts.Env["NODE_EXTRA_CA_CERTS"]; v != "" {
		return v
	}
	data, err := os.ReadFile(settingsFile)
	if err != nil {
		return ""
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return ""
	}
	envRaw, ok := top["env"]
	if !ok {
		return ""
	}
	var env map[string]string
	if err := json.Unmarshal(envRaw, &env); err != nil {
		return ""
	}
	return env["NODE_EXTRA_CA_CERTS"]
}

func currentNECCPi(opts Options, profile string) string {
	if v := opts.Env["NODE_EXTRA_CA_CERTS"]; v != "" {
		return v
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export NODE_EXTRA_CA_CERTS=") {
			v := strings.TrimPrefix(line, "export NODE_EXTRA_CA_CERTS=")
			v = strings.Trim(v, `"'`)
			return v
		}
	}
	return ""
}

func acceptsCATrust(value, caPath string) bool {
	if value == caPath {
		return true
	}
	caData, err := os.ReadFile(caPath)
	if err != nil {
		return false
	}
	bundleData, err := os.ReadFile(value)
	if err != nil {
		return false
	}
	return containsCertificate(bundleData, caData)
}

func containsCertificate(bundle, ca []byte) bool {
	for _, c := range parseCerts(ca) {
		for _, b := range parseCerts(bundle) {
			if c.Equal(b) {
				return true
			}
		}
	}
	return false
}

func parseCerts(pemData []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, pemData = pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			certs = append(certs, c)
		}
	}
	return certs
}

func bundleConflictError(current, caPath, source string) error {
	bundlePath := caPath[:len(caPath)-len(".pem")] + "-bundle.pem"
	return fmt.Errorf(`fail ca-trust NODE_EXTRA_CA_CERTS is %s; run: cat %q %q > %q; then set NODE_EXTRA_CA_CERTS to %s in %s`,
		current, current, caPath, bundlePath, bundlePath, source)
}

func caFilePath(opts Options) string {
	return opts.CfgDir + "/ca.pem"
}
