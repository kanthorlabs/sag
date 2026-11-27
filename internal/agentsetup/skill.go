package agentsetup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kanthorlabs/sag/internal/stepline"
)

const skillMarker = "<!-- managed by sag agent setup -->"

func installSkill(ctx context.Context, opts Options) error {
	out := stepline.New(agentOut)
	paths := harnessPathsFromOpts(opts)
	skillFile := paths.SkillFile

	if err := os.MkdirAll(filepath.Dir(skillFile), 0o700); err != nil {
		return err
	}

	content := buildSkillContent(opts.Sub)

	existing, err := os.ReadFile(skillFile)
	if err == nil {
		if !bytes.Contains(existing, []byte(skillMarker)) {
			out.Line(stepline.StatusSkip, "skill", "marker absent")
			return nil
		}
	}

	if err := os.WriteFile(skillFile, content, 0o644); err != nil {
		return err
	}
	out.Line(stepline.StatusOK, "skill", "")
	return nil
}

func buildSkillContent(sub string) []byte {
	content := string(skillMD)
	content = strings.ReplaceAll(content, "--agent <sub>", "--agent "+sub)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return []byte(content)
}
