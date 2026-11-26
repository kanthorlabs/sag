package dxh

import (
	"os"
	"path/filepath"
)

const systemPATH = "/usr/bin:/bin:/usr/sbin:/sbin"

func TestPATH(binDir string) string {
	return binDir + ":" + systemPATH
}

func AddRealProgramLink(linkDir, name, target string) error {
	link := filepath.Join(linkDir, name)
	_ = os.Remove(link)
	return os.Symlink(target, link)
}
