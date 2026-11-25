package secret

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func writeSecret(t *testing.T, dir, name string, value []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, value, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertNoValue(t *testing.T, err error, values ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	for _, v := range values {
		if v != "" && strings.Contains(err.Error(), v) {
			t.Fatalf("error holds the secret value: %v", err)
		}
	}
}

func TestAC02_RefusesMode0444(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, "sag-main", []byte(testValue), 0o444)

	creds, err := Load(dir, os.Getuid(), []string{"sag-main"})
	assertNoValue(t, err, testValue)
	if creds != nil {
		t.Fatal("Load returned credentials with an error")
	}
	if !strings.Contains(err.Error(), "sag-main") {
		t.Fatalf("error does not name the secret: %v", err)
	}
}

func TestRefusesModeOtherThan0400(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o440, 0o404, 0o640, 0o200, 0o000, 0o500, 0o400 | os.ModeSetuid, 0o400 | os.ModeSticky} {
		dir := t.TempDir()
		path := writeSecret(t, dir, "s", []byte(testValue), mode)
		if err := CheckFile(path, os.Getuid()); err == nil {
			t.Fatalf("CheckFile accepted mode %v", mode)
		}
		if _, err := Load(dir, os.Getuid(), []string{"s"}); err == nil {
			t.Fatalf("Load accepted mode %v", mode)
		}
	}
}

func TestAC02_RefusesShortMasterKey(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		raw := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xa5}, n))
		dir := t.TempDir()
		writeSecret(t, dir, MasterKeyName, []byte(raw), 0o400)
		creds, err := Load(dir, os.Getuid(), []string{MasterKeyName})
		if err != nil {
			t.Fatal(err)
		}
		_, err = DecodeMasterKey(creds[MasterKeyName])
		assertNoValue(t, err, raw)
	}
}

func TestDecodeMasterKey(t *testing.T) {
	master := bytes.Repeat([]byte{0x5a}, MasterKeySize)
	encoded := base64.StdEncoding.EncodeToString(master)
	for _, raw := range []string{encoded, encoded + "\n", encoded + "\r\n", " " + encoded + " \n"} {
		got, err := DecodeMasterKey(NewCredential(MasterKeyName, []byte(raw)))
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if !bytes.Equal(got, master) {
			t.Fatalf("%q: wrong key", raw)
		}
	}
}

func TestDecodeMasterKeyRejectsInvalid(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x5a}, MasterKeySize))
	for _, raw := range []string{
		"",
		"not base64 at all!",
		encoded[:len(encoded)-1],
		base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0x5a}, MasterKeySize)),
		base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, MasterKeySize)),
		string(bytes.Repeat([]byte{0x5a}, MasterKeySize)),
	} {
		_, err := DecodeMasterKey(NewCredential(MasterKeyName, []byte(raw)))
		assertNoValue(t, err, raw)
	}
}

func TestRefusesWrongOwner(t *testing.T) {
	dir := t.TempDir()
	path := writeSecret(t, dir, "sag-main", []byte(testValue), 0o400)
	other := os.Getuid() + 1

	assertNoValue(t, CheckFile(path, other), testValue)
	creds, err := Load(dir, other, []string{"sag-main"})
	assertNoValue(t, err, testValue)
	if creds != nil {
		t.Fatal("Load returned credentials with an error")
	}
}

func TestRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := writeSecret(t, t.TempDir(), "real", []byte(testValue), 0o400)
	link := filepath.Join(dir, "sag-main")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	assertNoValue(t, CheckFile(link, os.Getuid()), testValue)
	_, err := Load(dir, os.Getuid(), []string{"sag-main"})
	assertNoValue(t, err, testValue)
}

func TestRefusesNonRegular(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a-dir")
	if err := os.Mkdir(sub, 0o400); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "a-fifo")
	if err := syscall.Mkfifo(fifo, 0o400); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a-dir", "a-fifo"} {
		if err := CheckFile(filepath.Join(dir, name), os.Getuid()); err == nil {
			t.Fatalf("CheckFile accepted %s", name)
		}
		if _, err := Load(dir, os.Getuid(), []string{name}); err == nil {
			t.Fatalf("Load accepted %s", name)
		}
	}
}

func TestRefusesMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(dir, os.Getuid(), []string{"absent"}); err == nil {
		t.Fatal("Load accepted a missing secret")
	}
}

func TestRefusesNameOutsideDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "secrets")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSecret(t, parent, "outside", []byte(testValue), 0o400)
	for _, name := range []string{"../outside", "", ".", "..", "a/b", "/etc/passwd"} {
		_, err := Load(dir, os.Getuid(), []string{name})
		assertNoValue(t, err, testValue)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, "sag-main", []byte(testValue), 0o400)
	writeSecret(t, dir, "sag-other", []byte("other\n"), 0o400)
	writeSecret(t, dir, "unused", []byte("x"), 0o644)

	creds, err := Load(dir, os.Getuid(), []string{"sag-main", "sag-other", "sag-main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Fatalf("got %d credentials, want 2", len(creds))
	}
	if c := creds["sag-main"]; c.Name() != "sag-main" || string(c.Reveal()) != testValue {
		t.Fatal("sag-main has the wrong name or value")
	}
	if string(creds["sag-other"].Reveal()) != "other\n" {
		t.Fatal("Load changed the value of sag-other")
	}
	if err := CheckFile(filepath.Join(dir, "sag-main"), os.Getuid()); err != nil {
		t.Fatal(err)
	}
}
