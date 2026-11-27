package masterkey

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/kanthorlabs/sag/internal/podman"
	"github.com/kanthorlabs/sag/internal/secret"
	"github.com/kanthorlabs/sag/internal/stepline"
)

func Ensure(ctx context.Context, pm podman.Command, out stepline.Writer) (changed bool, err error) {
	_, err = pm.Run(ctx, nil, "secret", "inspect", secret.MasterKeyName)
	if err == nil {
		out.Line(stepline.StatusOK, "master-key", "")
		return false, nil
	}
	if !podman.SecretAbsent(err) {
		return false, err
	}

	raw := make([]byte, secret.MasterKeySize)
	if _, err := rand.Read(raw); err != nil {
		return false, err
	}
	value := base64.StdEncoding.EncodeToString(raw)

	if _, err := pm.Run(ctx, bytes.NewReader([]byte(value)), "secret", "create", secret.MasterKeyName, "-"); err != nil {
		return false, err
	}
	out.Line(stepline.StatusInstall, "master-key", "")
	return true, nil
}
