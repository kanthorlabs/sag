package agentsetup

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeepsBigIntegerText(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	input := `{"other":9007199254740993,"key":"old"}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, []string{"key"}, "new"); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	data, _ := os.ReadFile(path)
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(got["other"]) != "9007199254740993" {
		t.Errorf("other = %s; want 9007199254740993", got["other"])
	}
}

func TestKeepsUnicodeEscape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	input := `{"other":"\u0041nother","key":"old"}`
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, []string{"key"}, "new"); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) == "" {
		t.Fatal("empty output")
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(got["other"]) != `"\u0041nother"` {
		t.Errorf("other = %s; want \\u0041 literal", got["other"])
	}
}

func TestNoHTMLEscape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, []string{"key"}, "<value>&more"); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<value>") {
		t.Errorf("output %q: < was escaped; want literal <", data)
	}
	if !strings.Contains(string(data), "&more") {
		t.Errorf("output %q: & was escaped; want literal &", data)
	}
}

func TestSymlinkTargetWritten(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	if err := os.WriteFile(target, []byte(`{"key":"old"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(link, []string{"key"}, "new"); err != nil {
		t.Fatalf("writeJSON via symlink: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("symlink was removed")
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		t.Error("link is no longer a symlink")
	}
	data, _ := os.ReadFile(target)
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if got["key"] != "new" {
		t.Errorf("key = %q; want new", got["key"])
	}
}

func TestModeKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	if err := os.WriteFile(path, []byte(`{"key":"old"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(path, []string{"key"}, "new"); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o777 != 0o640 {
		t.Errorf("mode = %o; want 640", fi.Mode()&0o777)
	}
}

func TestNewFileMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")
	if err := writeJSON(path, []string{"key"}, "value"); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o777 != 0o600 {
		t.Errorf("mode = %o; want 600", fi.Mode()&0o777)
	}
}
