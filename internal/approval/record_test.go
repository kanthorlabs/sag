package approval

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	testKey  = bytes.Repeat([]byte{0x5a}, 32)
	otherKey = bytes.Repeat([]byte{0x6b}, 32)
	testNow  = time.Date(2026, 11, 25, 12, 30, 45, 123456789, time.UTC)
)

func savedStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), DirName)
	s := NewStore(dir, testKey)
	fp, err := Fingerprint(testTarget(), testObjects())
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if _, err := s.Save("ext", fp, testObjects(), testNow); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return s, dir, fp
}

func recordPath(dir, resource string) string {
	return filepath.Join(dir, resource+".json")
}

func editRecord(t *testing.T, dir string, edit func(map[string]any)) {
	t.Helper()
	path := recordPath(dir, "ext")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	edit(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("encode record: %v", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatalf("write record: %v", err)
	}
}

func TestMACValid(t *testing.T) {
	s, dir, fp := savedStore(t)
	if err := s.Verify("ext", fp); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	raw, err := os.ReadFile(recordPath(dir, "ext"))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	for _, k := range []string{"resource", "fingerprint", "objects", "approved_at", "mac"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("record has no field %q: %s", k, raw)
		}
	}
	if len(m) != 5 {
		t.Fatalf("record has %d fields; want 5: %s", len(m), raw)
	}
	var mac string
	if err := json.Unmarshal(m["mac"], &mac); err != nil {
		t.Fatalf("decode mac: %v", err)
	}
	if b, err := hex.DecodeString(mac); err != nil || len(b) != 32 || mac != strings.ToLower(mac) {
		t.Fatalf("mac = %q; want 64 lowercase hex digits", mac)
	}
}

func TestMACSaveReturnsRecord(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, testKey)
	fp, _ := Fingerprint(testTarget(), testObjects())
	rec, err := s.Save("ext", fp, testObjects(), testNow)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if rec.Resource != "ext" || rec.Fingerprint != fp || !rec.ApprovedAt.Equal(testNow) || rec.MAC == "" {
		t.Fatalf("Save returned %+v", rec)
	}
	if rec.Objects.Extensions[0].Name != "dblink" || rec.Objects.Languages[0] != "plperlu" {
		t.Fatalf("Save returned unsorted objects %+v", rec.Objects)
	}
}

func TestMACChangeRefused(t *testing.T) {
	edits := map[string]func(map[string]any){
		"mac byte": func(m map[string]any) {
			mac := []byte(m["mac"].(string))
			if mac[0] == '0' {
				mac[0] = '1'
			} else {
				mac[0] = '0'
			}
			m["mac"] = string(mac)
		},
		"mac not hex":    func(m map[string]any) { m["mac"] = "zz" },
		"mac empty":      func(m map[string]any) { m["mac"] = "" },
		"mac missing":    func(m map[string]any) { delete(m, "mac") },
		"approved_at":    func(m map[string]any) { m["approved_at"] = "2027-01-01T00:00:00Z" },
		"objects":        func(m map[string]any) { m["objects"] = map[string]any{"extensions": []any{}, "languages": []any{}} },
		"fingerprint":    func(m map[string]any) { m["fingerprint"] = strings.Repeat("0", 64) },
		"resource field": func(m map[string]any) { m["resource"] = "other" },
		"unknown field":  func(m map[string]any) { m["extra"] = true },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			s, dir, fp := savedStore(t)
			editRecord(t, dir, edit)
			err := s.Verify("ext", fp)
			if err == nil {
				t.Fatal("Verify accepted a changed record")
			}
			if errors.Is(err, ErrNotApproved) {
				t.Fatalf("Verify = %v; want a refusal that is not ErrNotApproved", err)
			}
		})
	}
}

func TestMACChangedByteIsInvalidMAC(t *testing.T) {
	s, dir, fp := savedStore(t)
	editRecord(t, dir, func(m map[string]any) {
		mac := []byte(m["mac"].(string))
		mac[len(mac)-1] ^= 0x01
		m["mac"] = string(mac)
	})
	if err := s.Verify("ext", fp); !errors.Is(err, ErrInvalidMAC) {
		t.Fatalf("Verify = %v; want ErrInvalidMAC", err)
	}
}

func TestMACOtherKeyRefused(t *testing.T) {
	_, dir, fp := savedStore(t)
	if err := NewStore(dir, otherKey).Verify("ext", fp); !errors.Is(err, ErrInvalidMAC) {
		t.Fatalf("Verify with another key = %v; want ErrInvalidMAC", err)
	}
}

func TestMACNoKeyRefused(t *testing.T) {
	_, dir, fp := savedStore(t)
	for _, key := range [][]byte{nil, {}, testKey[:16]} {
		if err := NewStore(dir, key).Verify("ext", fp); err == nil {
			t.Fatalf("Verify with a %d-byte key accepted the record", len(key))
		}
		if _, err := NewStore(t.TempDir(), key).Save("ext", fp, testObjects(), testNow); err == nil {
			t.Fatalf("Save with a %d-byte key wrote a record", len(key))
		}
	}
}

func TestMACFingerprintMismatch(t *testing.T) {
	s, _, _ := savedStore(t)
	o := testObjects()
	o.Extensions[0].Version = "1.1"
	changed, err := Fingerprint(testTarget(), o)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if err := s.Verify("ext", changed); !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("Verify = %v; want ErrFingerprintMismatch", err)
	}
	if err := s.Verify("ext", ""); !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("Verify(empty fingerprint) = %v; want ErrFingerprintMismatch", err)
	}
}

func TestMACRecordOfOtherResourceRefused(t *testing.T) {
	s, dir, fp := savedStore(t)
	raw, err := os.ReadFile(recordPath(dir, "ext"))
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	if err := os.WriteFile(recordPath(dir, "other"), raw, 0o600); err != nil {
		t.Fatalf("write copy: %v", err)
	}
	if err := s.Verify("other", fp); err == nil || errors.Is(err, ErrNotApproved) {
		t.Fatalf("Verify(other) with the record of ext = %v; want a refusal", err)
	}
}

func TestMACNoRecord(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), DirName), testKey)
	if err := s.Verify("ext", strings.Repeat("a", 64)); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("Verify without a record = %v; want ErrNotApproved", err)
	}
}

func TestFileMode0600(t *testing.T) {
	_, dir, _ := savedStore(t)
	info, err := os.Lstat(recordPath(dir, "ext"))
	if err != nil {
		t.Fatalf("stat record: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode = %v; want a regular file with mode 0600", info.Mode())
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Geteuid() {
		t.Fatalf("record owner is not the process user %d", os.Geteuid())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("store dir has %d entries; want only the record", len(entries))
	}
}

func TestFileModeSaveReplacesRecord(t *testing.T) {
	s, dir, _ := savedStore(t)
	if err := os.Chmod(recordPath(dir, "ext"), 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	o := Objects{Languages: []string{"plperlu"}}
	fp, _ := Fingerprint(testTarget(), o)
	if _, err := s.Save("ext", fp, o, testNow); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Lstat(recordPath(dir, "ext"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("replaced record mode = %v, %v; want 0600", info, err)
	}
	if err := s.Verify("ext", fp); err != nil {
		t.Fatalf("Verify replaced record: %v", err)
	}
}

func TestFileModeRefusesWideMode(t *testing.T) {
	for _, mode := range []fs.FileMode{0o644, 0o640, 0o400, 0o700} {
		s, dir, fp := savedStore(t)
		if err := os.Chmod(recordPath(dir, "ext"), mode); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		if err := s.Verify("ext", fp); err == nil || errors.Is(err, ErrNotApproved) {
			t.Fatalf("Verify of a record with mode %v = %v; want a refusal", mode, err)
		}
	}
}

func TestFileModeRefusesSymlink(t *testing.T) {
	s, dir, fp := savedStore(t)
	real := filepath.Join(t.TempDir(), "real.json")
	if err := os.Rename(recordPath(dir, "ext"), real); err != nil {
		t.Fatalf("move record: %v", err)
	}
	if err := os.Symlink(real, recordPath(dir, "ext")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := s.Verify("ext", fp); err == nil || errors.Is(err, ErrNotApproved) {
		t.Fatalf("Verify through a symlink = %v; want a refusal", err)
	}
}

func TestFileModeRefusesSymlinkDir(t *testing.T) {
	_, dir, fp := savedStore(t)
	link := filepath.Join(t.TempDir(), "approvals-link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	s := NewStore(link, testKey)
	if err := s.Verify("ext", fp); err == nil || errors.Is(err, ErrNotApproved) {
		t.Fatalf("Verify in a symlinked dir = %v; want a refusal", err)
	}
	if _, err := s.Save("ext", fp, testObjects(), testNow); err == nil {
		t.Fatal("Save wrote into a symlinked dir")
	}
}

func TestFileModeRefusesNonRegular(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, testKey)
	if err := os.Mkdir(recordPath(dir, "ext"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := s.Verify("ext", strings.Repeat("a", 64)); err == nil || errors.Is(err, ErrNotApproved) {
		t.Fatalf("Verify of a directory = %v; want a refusal", err)
	}
}

func TestFileModeRefusesBadResourceName(t *testing.T) {
	s := NewStore(t.TempDir(), testKey)
	fp := strings.Repeat("a", 64)
	for _, name := range []string{"", "../ext", "a/b", ".", "..", "Ext", "ext.json", "-ext"} {
		if _, err := s.Save(name, fp, Objects{}, testNow); err == nil {
			t.Fatalf("Save(%q) wrote a record", name)
		}
		if err := s.Verify(name, fp); err == nil {
			t.Fatalf("Verify(%q) accepted", name)
		}
		if err := s.Revoke(name); err == nil {
			t.Fatalf("Revoke(%q) returned no error", name)
		}
	}
}

func TestFileModeRevokeRemoves(t *testing.T) {
	s, dir, fp := savedStore(t)
	if err := s.Revoke("ext"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := os.Lstat(recordPath(dir, "ext")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("record still exists after Revoke: %v", err)
	}
	if err := s.Verify("ext", fp); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("Verify after Revoke = %v; want ErrNotApproved", err)
	}
	if err := s.Revoke("ext"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("second Revoke = %v; want ErrNotApproved", err)
	}
}

func TestFileModeRevokeWithoutKey(t *testing.T) {
	_, dir, _ := savedStore(t)
	if err := NewStore(dir, nil).Revoke("ext"); err != nil {
		t.Fatalf("Revoke without a key: %v", err)
	}
}
