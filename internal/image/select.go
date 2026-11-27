package image

import "github.com/kanthorlabs/sag/internal/deploy"

const (
	imageName    = "docker.io/kanthorlabs/sag"
	devReference = imageName + ":main"
)

func productReference() (string, bool) {
	if deploy.ImageDigest != "" {
		return imageName + "@" + deploy.ImageDigest, false
	}
	return devReference, true
}
