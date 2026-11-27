package dxh

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const keyScanMaxSize = 1 << 20

var keyScanTimeout = 5 * time.Minute

var keyScanDevOf = func(st *syscall.Stat_t) int64 {
	return int64(st.Dev)
}

var errMatchFound = errors.New("match found")

type KeyScanResult struct {
	MatchFound bool
	MatchPath  string
	DeniedDirs int
	TimedOut   bool
}

type CanaryParts struct {
	Password string
	Host     string
	User     string
	Database string
}

var ActiveCanaryParts CanaryParts

var ActiveCanaryTranscript []byte

func SearchStrings(sources []string, needles []string) []string {
	var found []string
	for _, needle := range needles {
		for _, src := range sources {
			if strings.Contains(src, needle) {
				found = append(found, needle)
				break
			}
		}
	}
	return found
}

func SearchNamedParts(sources map[string]string, parts map[string]string) []string {
	var findings []string
	for partName, value := range parts {
		if value == "" {
			continue
		}
		for srcName, content := range sources {
			if strings.Contains(content, value) {
				findings = append(findings, partName+" in "+srcName)
			}
		}
	}
	return findings
}

func ScanForMatchingKey(ctx context.Context, home, tmpDir string, pubKey *ecdsa.PublicKey, groupStart time.Time) (KeyScanResult, error) {
	scanCtx, cancel := context.WithTimeout(ctx, keyScanTimeout)
	defer cancel()

	homeSt, err := os.Stat(home)
	if err != nil {
		return KeyScanResult{}, err
	}
	homeSys := homeSt.Sys().(*syscall.Stat_t)
	homeDev := keyScanDevOf(homeSys)

	var result KeyScanResult

	for _, root := range []string{home, tmpDir} {
		if root == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			resolved = root
		}
		if err := scanDir(scanCtx, resolved, homeDev, pubKey, groupStart, &result); err != nil {
			if errors.Is(err, errMatchFound) {
				return result, nil
			}
			if errors.Is(err, context.DeadlineExceeded) {
				result.TimedOut = true
				return result, errors.New("key scan timed out after 5 minutes")
			}
			return result, err
		}
		if result.MatchFound {
			return result, nil
		}
	}
	return result, nil
}

func scanDir(ctx context.Context, root string, homeDev int64, pubKey *ecdsa.PublicKey, groupStart time.Time, result *KeyScanResult) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err != nil {
			if os.IsPermission(err) {
				result.DeniedDirs++
				return fs.SkipDir
			}
			return nil
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return nil
			}
			if keyScanDevOf(st) != homeDev {
				return fs.SkipDir
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}

		if keyScanDevOf(st) != homeDev {
			return nil
		}

		ctime := ctimeOf(st)
		if ctime.Before(groupStart) {
			return nil
		}

		if info.Size() > keyScanMaxSize {
			return nil
		}

		match, err := fileContainsMatchingKey(path, pubKey)
		if err != nil || !match {
			return nil
		}
		result.MatchFound = true
		result.MatchPath = path
		return errMatchFound
	})
}

func fileContainsMatchingKey(path string, pubKey *ecdsa.PublicKey) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil
	}
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "EC PRIVATE KEY" && block.Type != "PRIVATE KEY" {
			continue
		}
		key, err := parsePrivateKey(block)
		if err != nil {
			continue
		}
		if matchesPublicKey(key, pubKey) {
			return true, nil
		}
	}
	return false, nil
}

func parsePrivateKey(block *pem.Block) (*ecdsa.PrivateKey, error) {
	switch block.Type {
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		ec, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an ECDSA key")
		}
		return ec, nil
	}
	return nil, errors.New("unknown block type")
}

func matchesPublicKey(priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) bool {
	return priv.PublicKey.X.Cmp(pub.X) == 0 && priv.PublicKey.Y.Cmp(pub.Y) == 0 && priv.PublicKey.Curve == pub.Curve
}
