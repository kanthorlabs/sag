package harness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

const busyboxSource = "/bin/busybox"

func busyboxContainerName(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return "sag-e2e-busybox-" + hex.EncodeToString(sum[:6])
}

func BusyboxMount(ctx context.Context, dir string) (path string, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	name := busyboxContainerName(dir)
	if _, err := podman(ctx, "create", "--name", name, BusyboxImage); err != nil {
		return "", err
	}
	defer func() {
		_, rmErr := podman(context.WithoutCancel(ctx), "rm", "-f", "-t", "0", name)
		err = errors.Join(err, rmErr)
	}()
	path = filepath.Join(dir, filepath.Base(busyboxSource))
	if _, err := podman(ctx, "cp", name+":"+busyboxSource, path); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}
