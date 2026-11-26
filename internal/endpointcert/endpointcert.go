package endpointcert

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/stepline"
	"github.com/kanthorlabs/sag/internal/testhook"
)

const (
	certSecretName   = "sag-tls-cert"
	keySecretName    = "sag-tls-key"
	defaultValidDays = 397
	renewThreshold   = 30 * 24 * time.Hour
	tailscaleCIDR    = "100.64.0.0/10"
)

var tailscaleNet *net.IPNet

func init() {
	_, tailscaleNet, _ = net.ParseCIDR(tailscaleCIDR)
}

type Config struct {
	CfgDir      string
	TailscaleIP string
}

func Ensure(ctx context.Context, pm podman.Command, cfg Config, now time.Time, out stepline.Writer) (changed bool, err error) {
	if cfg.TailscaleIP != "" {
		ip := net.ParseIP(cfg.TailscaleIP)
		if ip == nil || !tailscaleNet.Contains(ip) {
			return false, errors.New("TailscaleIP is not in 100.64.0.0/10")
		}
	}

	triggered, err := needsRenewal(ctx, pm, cfg, now)
	if err != nil {
		return false, err
	}
	if !triggered {
		out.Line(stepline.StatusOK, "endpoint-cert", "")
		return false, nil
	}

	if err := runSequence(ctx, pm, cfg, now, out); err != nil {
		return false, err
	}
	return true, nil
}

func needsRenewal(ctx context.Context, pm podman.Command, cfg Config, now time.Time) (bool, error) {
	_, errCert := pm.Run(ctx, nil, "secret", "inspect", certSecretName)
	_, errKey := pm.Run(ctx, nil, "secret", "inspect", keySecretName)
	if isAbsent(errCert) || isAbsent(errKey) {
		return true, nil
	}
	if errCert != nil {
		return false, errCert
	}
	if errKey != nil {
		return false, errKey
	}

	_, statNew := os.Stat(filepath.Join(cfg.CfgDir, "ca.pem.new"))
	if statNew == nil {
		return true, nil
	}

	caPEM, err := os.ReadFile(filepath.Join(cfg.CfgDir, "ca.pem"))
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	ca, err := parseCertFromPEM(caPEM)
	if err != nil {
		return true, nil
	}
	if ca.NotAfter.Sub(now) < renewThreshold {
		return true, nil
	}

	if cfg.TailscaleIP != "" {
		stored, err := readStoredTailscaleIP(cfg.CfgDir)
		if err != nil {
			return false, err
		}
		if stored != cfg.TailscaleIP {
			return true, nil
		}
	}

	return false, nil
}

func isAbsent(err error) bool {
	if err == nil {
		return false
	}
	var re *podman.RunError
	if errors.As(err, &re) {
		return re.ExitCode != 0
	}
	return false
}

func runSequence(ctx context.Context, pm podman.Command, cfg Config, now time.Time, out stepline.Writer) error {
	validDays := defaultValidDays
	if d := testhook.CertDays(); d > 0 {
		validDays = d
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	notBefore := now.Add(-time.Hour)
	notAfter := now.Add(time.Duration(validDays) * 24 * time.Hour)

	caTemplate := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "SAG CA"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}

	serial2, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	if cfg.TailscaleIP != "" {
		ips = append(ips, net.ParseIP(cfg.TailscaleIP))
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: serial2,
		Subject:      pkix.Name{CommonName: "SAG"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  ips,
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}

	caPEMBuf := new(bytes.Buffer)
	if err := pem.Encode(caPEMBuf, &pem.Block{Type: "CERTIFICATE", Bytes: caDER}); err != nil {
		return err
	}

	serverCertBuf := new(bytes.Buffer)
	if err := pem.Encode(serverCertBuf, &pem.Block{Type: "CERTIFICATE", Bytes: serverDER}); err != nil {
		return err
	}

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		return err
	}
	serverKeyBuf := new(bytes.Buffer)
	if err := pem.Encode(serverKeyBuf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER}); err != nil {
		return err
	}

	newPath := filepath.Join(cfg.CfgDir, "ca.pem.new")
	if err := os.WriteFile(newPath, caPEMBuf.Bytes(), 0644); err != nil {
		return err
	}

	for _, name := range []string{certSecretName, keySecretName} {
		if _, err := pm.Run(ctx, nil, "secret", "inspect", name); err == nil {
			if _, err := pm.Run(ctx, nil, "secret", "rm", name); err != nil {
				return err
			}
		}
	}

	if _, err := pm.Run(ctx, bytes.NewReader(serverCertBuf.Bytes()), "secret", "create", certSecretName, "-"); err != nil {
		return err
	}
	if _, err := pm.Run(ctx, bytes.NewReader(serverKeyBuf.Bytes()), "secret", "create", keySecretName, "-"); err != nil {
		return err
	}

	if testhook.FailCertRenewal() {
		return errors.New("SAG_TEST_FAIL_CERT_RENEWAL")
	}

	pemPath := filepath.Join(cfg.CfgDir, "ca.pem")
	if err := os.Rename(newPath, pemPath); err != nil {
		return err
	}

	if err := updateTailscaleIP(cfg.CfgDir, cfg.TailscaleIP); err != nil {
		return err
	}

	out.Line(stepline.StatusInstall, "endpoint-cert", "")
	return nil
}

func parseCertFromPEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	return x509.ParseCertificate(block.Bytes)
}

func readStoredTailscaleIP(cfgDir string) (string, error) {
	path := filepath.Join(cfgDir, "answers.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", nil
	}
	v, ok := raw["tailscale_ip"]
	if !ok {
		return "", nil
	}
	var ip string
	if err := json.Unmarshal(v, &ip); err != nil {
		return "", nil
	}
	return ip, nil
}

func updateTailscaleIP(cfgDir, tailscaleIP string) error {
	path := filepath.Join(cfgDir, "answers.json")
	raw := make(map[string]json.RawMessage)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
	}
	if tailscaleIP != "" {
		b, err := json.Marshal(tailscaleIP)
		if err != nil {
			return err
		}
		raw["tailscale_ip"] = b
	} else {
		delete(raw, "tailscale_ip")
	}
	if len(raw) == 0 {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
}
