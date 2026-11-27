package agentsetup

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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
	env := opts.Env["NODE_EXTRA_CA_CERTS"]
	if env != "" && !acceptsCATrust(env, caPath) {
		return newCATrustConflictError(env, caPath, "the environment")
	}
	settings := currentNECCFromSettings(paths.SettingsFile)
	if settings != "" && !acceptsCATrust(settings, caPath) {
		return newCATrustConflictError(settings, caPath, paths.SettingsFile)
	}
	if settings == "" {
		if err := writeJSON(paths.SettingsFile, []string{"env", "NODE_EXTRA_CA_CERTS"}, caPath); err != nil {
			return err
		}
	}
	out.Line(stepline.StatusOK, "ca-trust", "")
	return nil
}

func applyCATrustPi(opts Options, caPath string, out stepline.Writer) error {
	shell := opts.Env["SHELL"]
	zdotdir := opts.Env["ZDOTDIR"]
	profile, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
	if !ok {
		out.Line(stepline.StatusSkip, "ca-trust", "")
		return nil
	}
	env := opts.Env["NODE_EXTRA_CA_CERTS"]
	if env != "" && !acceptsCATrust(env, caPath) {
		return newCATrustConflictError(env, caPath, "the environment")
	}
	current := currentNECCFromProfile(profile)
	if current != "" {
		if acceptsCATrust(current, caPath) {
			out.Line(stepline.StatusOK, "ca-trust", "")
			return nil
		}
		return newCATrustConflictError(current, caPath, profile)
	}
	out.Line(stepline.StatusSkip, "ca-trust", "")
	return nil
}

func currentNECCFromSettings(settingsFile string) string {
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

func currentNECCFromProfile(profile string) string {
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

func checkCATrustConflict(opts Options, paths HarnessPaths) string {
	caPath := caFilePath(opts)
	switch opts.Harness {
	case HarnessPi:
		shell := opts.Env["SHELL"]
		zdotdir := opts.Env["ZDOTDIR"]
		profile, ok := ShellProfile(opts.GOOS, shell, zdotdir, opts.Home)
		if !ok {
			return ""
		}
		env := opts.Env["NODE_EXTRA_CA_CERTS"]
		if env != "" && !acceptsCATrust(env, caPath) {
			return newCATrustConflictError(env, caPath, "the environment").Error()
		}
		current := currentNECCFromProfile(profile)
		if current != "" && !acceptsCATrust(current, caPath) {
			return newCATrustConflictError(current, caPath, profile).Error()
		}
	default:
		if paths.SettingsFile == "" {
			return ""
		}
		env := opts.Env["NODE_EXTRA_CA_CERTS"]
		if env != "" && !acceptsCATrust(env, caPath) {
			return newCATrustConflictError(env, caPath, "the environment").Error()
		}
		settings := currentNECCFromSettings(paths.SettingsFile)
		if settings != "" && !acceptsCATrust(settings, caPath) {
			return newCATrustConflictError(settings, caPath, paths.SettingsFile).Error()
		}
	}
	return ""
}

func newCATrustConflictError(current, caPath, source string) *CATrustConflictError {
	bundlePath := caPath[:len(caPath)-len(".pem")] + "-bundle.pem"
	return &CATrustConflictError{
		Current:    current,
		CAPath:     caPath,
		BundlePath: bundlePath,
		Source:     source,
	}
}

func bundleConflictError(current, caPath, source string) error {
	return newCATrustConflictError(current, caPath, source)
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

func profileHasAcceptedValue(profile, caPath string) bool {
	data, err := os.ReadFile(profile)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "export NODE_EXTRA_CA_CERTS=") {
			v := strings.TrimPrefix(line, "export NODE_EXTRA_CA_CERTS=")
			v = strings.Trim(v, `"'`)
			if acceptsCATrust(v, caPath) {
				return true
			}
		}
	}
	return false
}

func caFilePath(opts Options) string {
	return opts.CfgDir + "/ca.pem"
}

func currentNECCClaudeCode(opts Options, settingsFile string) string {
	if v := opts.Env["NODE_EXTRA_CA_CERTS"]; v != "" {
		return v
	}
	return currentNECCFromSettings(settingsFile)
}

func currentNECCPi(opts Options, profile string) string {
	if v := opts.Env["NODE_EXTRA_CA_CERTS"]; v != "" {
		return v
	}
	return currentNECCFromProfile(profile)
}
