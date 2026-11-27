package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kanthorlabs/sag/internal/agentsetup"
)

var errCATrustConflict = errors.New("ca-trust conflict")

type harnessDet struct {
	harness agentsetup.Harness
	cfgDir  string
	sub     string
}

type agentRunState struct {
	detected   []harnessDet
	completed  []harnessDet
	caConflict bool
}

func agentSetupKey(harness agentsetup.Harness, cfgDir string) string {
	return string(harness) + ":" + cfgDir
}

func defaultEnvMap() map[string]string {
	m := make(map[string]string)
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func defaultHarnessInPATH(harness agentsetup.Harness, environ map[string]string) bool {
	name := string(harness)
	if harness == agentsetup.HarnessClaudeCode {
		name = "claude"
	}
	pathEnv := environ["PATH"]
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	for _, dir := range strings.Split(pathEnv, ":") {
		if dir == "" {
			continue
		}
		info, err := os.Stat(dir + "/" + name)
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func convertAgentRunError(err error) error {
	if err == nil {
		return nil
	}
	var catErr *agentsetup.CATrustConflictError
	if errors.As(err, &catErr) {
		return errCATrustConflict
	}
	var conflictErr *agentsetup.ConflictError
	if errors.As(err, &conflictErr) && strings.Contains(conflictErr.Error(), "ca-trust NODE_EXTRA_CA_CERTS") {
		return errCATrustConflict
	}
	return err
}

func harnessConfigDir(harness agentsetup.Harness, environ map[string]string, home string) string {
	var dir string
	switch harness {
	case agentsetup.HarnessPi:
		dir = agentsetup.PiPaths(environ["PI_CODING_AGENT_DIR"], home).Dir
	default:
		dir = agentsetup.ClaudeCodePaths(environ["CLAUDE_CONFIG_DIR"], home).Dir
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func planEnv(environ map[string]string) map[string]string {
	env := make(map[string]string, len(environ))
	for k, v := range environ {
		env[k] = v
	}
	delete(env, "NODE_EXTRA_CA_CERTS")
	return env
}

func (d Deps) detectHarnesses(environ map[string]string, home string) []harnessDet {
	var result []harnessDet
	for _, h := range []agentsetup.Harness{agentsetup.HarnessClaudeCode, agentsetup.HarnessPi} {
		if !d.HarnessInPATH(h, environ) {
			continue
		}
		cfgDir := harnessConfigDir(h, environ, home)
		result = append(result, harnessDet{harness: h, cfgDir: cfgDir, sub: string(h)})
	}
	return result
}

func (d Deps) planAgentSetup(ctx context.Context, s *State, fresh agentSetupFresh, environ map[string]string, home string) error {
	if d.HarnessInPATH == nil {
		return nil
	}
	detected := d.detectHarnesses(environ, home)
	if len(detected) == 0 {
		return nil
	}
	if s.Answers.AgentSetup == nil {
		s.Answers.AgentSetup = make(map[string]string)
	}
	for _, det := range detected {
		key := agentSetupKey(det.harness, det.cfgDir)
		flag := s.Flags.AgentSetup[string(det.harness)]
		stored := s.Answers.AgentSetup[key]
		if stored == "" {
			stored = fresh[key]
		}
		answer, err := d.ResolveYesNo(s.Asker, flag, stored,
			fmt.Sprintf("Set up SAG for %s in %s? [y/N] ", det.harness, det.cfgDir),
			"--agent-setup")
		if err != nil {
			return err
		}
		s.Answers.AgentSetup[key] = answer
	}
	if s.Lock != nil || d.PlanOnly == nil {
		return nil
	}
	pe := planEnv(environ)
	for _, det := range detected {
		key := agentSetupKey(det.harness, det.cfgDir)
		if s.Answers.AgentSetup[key] != "yes" {
			continue
		}
		opts := d.buildAgentOptions(det, s, pe)
		plan, err := d.PlanOnly(ctx, opts)
		if err != nil {
			var catErr *agentsetup.CATrustConflictError
			if errors.As(err, &catErr) {
				continue
			}
			var conflictErr *agentsetup.ConflictError
			if errors.As(err, &conflictErr) && strings.Contains(conflictErr.Error(), "ca-trust NODE_EXTRA_CA_CERTS") {
				continue
			}
			return err
		}
		if s.AgentSetupAnswers == nil {
			s.AgentSetupAnswers = make(map[string]agentsetup.ResolvedAnswers)
		}
		s.AgentSetupAnswers[key] = plan.Answers
	}
	return nil
}

type agentSetupFresh map[string]string

func (d Deps) runAgentSetup(ctx context.Context, s *State, as *agentRunState) error {
	if d.HarnessInPATH == nil || d.AgentRun == nil {
		return nil
	}
	environ := make(map[string]string)
	if d.Environ != nil {
		environ = d.Environ()
	}
	detected := d.detectHarnesses(environ, s.Home)
	as.detected = detected

	for _, det := range detected {
		key := agentSetupKey(det.harness, det.cfgDir)
		if s.Answers.AgentSetup[key] != "yes" {
			continue
		}
		opts := d.buildAgentOptions(det, s, environ)
		err := d.AgentRun(ctx, opts)
		if err == nil {
			as.completed = append(as.completed, det)
			continue
		}
		if errors.Is(err, errCATrustConflict) {
			as.caConflict = true
			continue
		}
		return err
	}

	return nil
}

func (d Deps) buildAgentOptions(det harnessDet, s *State, environ map[string]string) agentsetup.Options {
	var denyRules, shellProfile, guard string
	switch det.harness {
	case agentsetup.HarnessClaudeCode:
		denyRules = s.Flags.DenyRules
	case agentsetup.HarnessPi:
		shellProfile = s.Flags.ShellProfile
		guard = s.Flags.Guard
	}
	if s.AgentSetupAnswers != nil {
		if ra, ok := s.AgentSetupAnswers[agentSetupKey(det.harness, det.cfgDir)]; ok {
			if denyRules == "" {
				denyRules = ra.DenyRules
			}
			if shellProfile == "" {
				shellProfile = ra.ShellProfile
			}
			if guard == "" {
				guard = ra.Guard
			}
		}
	}
	return agentsetup.Options{
		Harness:      det.harness,
		Sub:          det.sub,
		URL:          "https://localhost:8443/mcp",
		CfgDir:       s.Paths.Cfg,
		PodmanPath:   s.PodmanPath,
		Env:          environ,
		Home:         s.Home,
		GOOS:         d.GOOS(),
		DenyRules:    denyRules,
		ShellProfile: shellProfile,
		Guard:        guard,
		UnderLock:    true,
	}
}
