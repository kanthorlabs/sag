package hoststate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCfgDirXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", t.TempDir())

	p, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	want := filepath.Join(dir, "sag")
	if p.Cfg != want {
		t.Fatalf("Cfg = %q; want %q", p.Cfg, want)
	}
}

func TestCfgMode0700(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:         filepath.Join(dir, "sag"),
		SAGYaml:     filepath.Join(dir, "sag", "sag.yaml"),
		CAPem:       filepath.Join(dir, "sag", "ca.pem"),
		AnswersJSON: filepath.Join(dir, "sag", "answers.json"),
		Lock:        filepath.Join(dir, "sag", "lock"),
	}

	created, err := EnsureCfg(p)
	if err != nil {
		t.Fatalf("EnsureCfg: %v", err)
	}
	if !created {
		t.Fatal("EnsureCfg returned created=false for absent dir")
	}

	info, err := os.Stat(p.Cfg)
	if err != nil {
		t.Fatalf("Stat cfg: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("cfg mode = %04o; want 0700", info.Mode().Perm())
	}

	created2, err := EnsureCfg(p)
	if err != nil {
		t.Fatalf("EnsureCfg second call: %v", err)
	}
	if created2 {
		t.Fatal("EnsureCfg returned created=true for existing dir")
	}
}

func TestLockHeldExit1(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:  dir,
		Lock: filepath.Join(dir, "lock"),
	}

	lock1, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("AcquireLock first: %v", err)
	}
	defer lock1.Release()

	_, err = AcquireLock(p)
	if err == nil {
		t.Fatal("AcquireLock succeeded on a held lock; want error")
	}
}

func TestAnswersMode0600(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:         dir,
		AnswersJSON: filepath.Join(dir, "answers.json"),
	}

	a := Answers{Autostart: "yes", PodmanPath: "/opt/podman/bin/podman"}
	if err := SaveAnswers(p, a); err != nil {
		t.Fatalf("SaveAnswers: %v", err)
	}

	info, err := os.Stat(p.AnswersJSON)
	if err != nil {
		t.Fatalf("Stat answers.json: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("answers.json mode = %04o; want 0600", info.Mode().Perm())
	}

	got, err := LoadAnswers(p)
	if err != nil {
		t.Fatalf("LoadAnswers: %v", err)
	}
	if got.Autostart != "yes" || got.PodmanPath != "/opt/podman/bin/podman" {
		t.Fatalf("LoadAnswers = %+v; want autostart yes, podman_path set", got)
	}
}

func TestLoadAnswersAbsentFileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := Paths{AnswersJSON: filepath.Join(dir, "absent.json")}

	got, err := LoadAnswers(p)
	if err != nil {
		t.Fatalf("LoadAnswers absent: %v", err)
	}
	if got.Autostart != "" || got.PodmanPath != "" || got.TailscaleIP != "" {
		t.Fatalf("LoadAnswers absent = %+v; want empty", got)
	}
}

func TestSaveAnswersAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:         dir,
		AnswersJSON: filepath.Join(dir, "answers.json"),
	}

	a := Answers{Autostart: "no"}
	if err := SaveAnswers(p, a); err != nil {
		t.Fatalf("SaveAnswers: %v", err)
	}

	raw, err := os.ReadFile(p.AnswersJSON)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var check map[string]any
	if err := json.Unmarshal(raw, &check); err != nil {
		t.Fatalf("answers.json is not valid JSON: %v", err)
	}
}

func TestPodmanFixedPathOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first", "podman")
	second := filepath.Join(dir, "second", "podman")

	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := podmanFixedPaths
	podmanFixedPaths = []string{first, second}
	defer func() { podmanFixedPaths = orig }()

	t.Setenv("PATH", t.TempDir())

	got, err := FindPodman("")
	if err != nil {
		t.Fatalf("FindPodman: %v", err)
	}
	if got != first {
		t.Fatalf("FindPodman = %q; want first path %q", got, first)
	}
}

func TestPodmanSearchOrder(t *testing.T) {
	dir := t.TempDir()
	fakePodman := filepath.Join(dir, "podman")
	if err := os.WriteFile(fakePodman, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+":/usr/bin:/bin")

	got, err := FindPodman("")
	if err != nil {
		t.Fatalf("FindPodman via PATH: %v", err)
	}
	if got != fakePodman {
		t.Fatalf("FindPodman = %q; want %q", got, fakePodman)
	}

	t.Setenv("PATH", "/usr/bin:/bin")

	dir2 := t.TempDir()
	fakePodman2 := filepath.Join(dir2, "podman")
	if err := os.WriteFile(fakePodman2, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got2, err := FindPodman(fakePodman2)
	if err != nil {
		t.Fatalf("FindPodman stored: %v", err)
	}
	if got2 != fakePodman2 {
		t.Fatalf("FindPodman stored = %q; want %q", got2, fakePodman2)
	}
}

func TestFindPodmanStoredMissingFallsBack(t *testing.T) {
	dir := t.TempDir()
	fakePodman := filepath.Join(dir, "podman")
	if err := os.WriteFile(fakePodman, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")

	got, err := FindPodman("/nonexistent/podman")
	if err != nil {
		t.Fatalf("FindPodman fallback: %v", err)
	}
	if got != fakePodman {
		t.Fatalf("FindPodman fallback = %q; want %q", got, fakePodman)
	}
}

func TestLockRelease(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:  dir,
		Lock: filepath.Join(dir, "lock"),
	}

	lock, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	lock2, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("AcquireLock after release: %v", err)
	}
	lock2.Release()
}

func TestLockFileNotRemoved(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:  dir,
		Lock: filepath.Join(dir, "lock"),
	}

	lock, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	lock.Release()

	if _, err := os.Stat(p.Lock); err != nil {
		t.Fatalf("lock file removed after Release; want it present: %v", err)
	}
}

func TestAcquireLockCreatesFileWithMode0600(t *testing.T) {
	dir := t.TempDir()
	p := Paths{
		Cfg:  dir,
		Lock: filepath.Join(dir, "lock"),
	}

	lock, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer lock.Release()

	info, err := os.Stat(p.Lock)
	if err != nil {
		t.Fatalf("Stat lock: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode = %04o; want 0600", info.Mode().Perm())
	}
}

func lockIsTaken(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	return err != nil
}

func TestEnsureCfgFixesModeOnExistingDir(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "sag")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := Paths{Cfg: cfgDir, Lock: filepath.Join(cfgDir, "lock")}

	created, err := EnsureCfg(p)
	if err != nil {
		t.Fatalf("EnsureCfg: %v", err)
	}
	if created {
		t.Fatal("EnsureCfg returned created=true for existing dir")
	}

	info, err := os.Stat(cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("cfg mode = %04o; want 0700", info.Mode().Perm())
	}
}
