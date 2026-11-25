package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var prdFields = []string{"ts", "request_id", "remote_addr", "sub", "jti", "auth_cause", "resource", "capability", "operation", "sql", "sql_sha256", "rendered_sha256", "decision", "reason", "rows", "bytes", "truncated", "duration_ms"}

func newWriter(t *testing.T) (*FileWriter, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), DirName)
	w, err := NewFileWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	return w, dir
}

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("file %s does not end with a newline", path)
	}
	var lines []map[string]any
	for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func allowRecord(ts time.Time) Record {
	sub, jti := "ac26", "01JTI"
	return Record{
		TS:             Timestamp(ts),
		RequestID:      "01REQ",
		RemoteAddr:     "10.0.0.1:5000",
		Sub:            &sub,
		JTI:            &jti,
		Resource:       "main",
		Capability:     "sql",
		Operation:      "query",
		SQL:            "SELECT 1",
		SQLSHA256:      "in-sha",
		RenderedSHA256: "out-sha",
		Decision:       DecisionAllow,
		Rows:           1,
		Bytes:          42,
		Truncated:      true,
		DurationMS:     7,
	}
}

func TestDailyFileMode0600(t *testing.T) {
	w, dir := newWriter(t)
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
	east := time.FixedZone("east", 10*3600)
	if err := w.Append(allowRecord(time.Date(2026, 11, 26, 8, 0, 0, 0, east))); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(allowRecord(time.Date(2026, 11, 25, 23, 59, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(allowRecord(time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v; want 0600", e.Name(), info.Mode().Perm())
		}
	}
	if strings.Join(names, ",") != "audit-2026-11-25.jsonl,audit-2026-11-26.jsonl" {
		t.Fatalf("files = %v", names)
	}
	if n := len(readLines(t, filepath.Join(dir, "audit-2026-11-25.jsonl"))); n != 2 {
		t.Fatalf("audit-2026-11-25.jsonl has %d lines; want 2", n)
	}
	if n := len(readLines(t, filepath.Join(dir, "audit-2026-11-26.jsonl"))); n != 1 {
		t.Fatalf("audit-2026-11-26.jsonl has %d lines; want 1", n)
	}
}

func TestAllFields(t *testing.T) {
	w, dir := newWriter(t)
	ts := time.Date(2026, 11, 25, 10, 0, 0, 7e6, time.UTC)
	if err := w.Append(allowRecord(ts)); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(AuthFailure(ts, "01AUTH", "10.0.0.2:6000", "expired")); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, filepath.Join(dir, "audit-2026-11-25.jsonl"))
	if len(lines) != 2 {
		t.Fatalf("lines = %d; want 2", len(lines))
	}
	for _, line := range lines {
		if len(line) != len(prdFields) {
			t.Fatalf("line has %d keys; want %d: %v", len(line), len(prdFields), line)
		}
		for _, f := range prdFields {
			if _, ok := line[f]; !ok {
				t.Fatalf("line has no key %q: %v", f, line)
			}
		}
	}
	want := map[string]any{
		"ts": "2026-11-25T10:00:00.007Z", "request_id": "01REQ", "remote_addr": "10.0.0.1:5000",
		"sub": "ac26", "jti": "01JTI", "auth_cause": nil, "resource": "main", "capability": "sql",
		"operation": "query", "sql": "SELECT 1", "sql_sha256": "in-sha", "rendered_sha256": "out-sha",
		"decision": "allow", "reason": "", "rows": 1.0, "bytes": 42.0, "truncated": true, "duration_ms": 7.0,
	}
	for k, v := range want {
		if lines[0][k] != v {
			t.Fatalf("allow %s = %#v; want %#v", k, lines[0][k], v)
		}
	}
	auth := lines[1]
	if auth["sub"] != nil || auth["jti"] != nil || auth["auth_cause"] != "expired" || auth["decision"] != "deny" || auth["reason"] != "unauthenticated" || auth["remote_addr"] != "10.0.0.2:6000" {
		t.Fatalf("auth failure line = %v", auth)
	}
}

func TestNoJWTNoValues(t *testing.T) {
	w, dir := newWriter(t)
	ts := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	if err := w.Append(allowRecord(ts)); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(AuthFailure(ts, "01AUTH", "10.0.0.2:6000", "unauthenticated")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "audit-2026-11-25.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("eyJ")) || bytes.Contains(data, []byte("Bearer")) {
		t.Fatalf("audit line holds a token: %s", data)
	}
	for _, line := range readLines(t, filepath.Join(dir, "audit-2026-11-25.jsonl")) {
		if _, ok := line["rows"].(float64); !ok {
			t.Fatalf("rows = %#v; want a count, not values", line["rows"])
		}
		if _, ok := line["bytes"].(float64); !ok {
			t.Fatalf("bytes = %#v; want a count", line["bytes"])
		}
	}
}

func TestAppendError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not apply")
	}
	dir := filepath.Join(t.TempDir(), DirName)
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	w, err := NewFileWriter(dir)
	if err != nil {
		t.Fatalf("NewFileWriter on a read-only directory: %v", err)
	}
	if err := w.Append(allowRecord(time.Now())); err == nil {
		t.Fatal("Append on a read-only directory: want error")
	}
}

func TestConcurrentAppendsKeepWholeLines(t *testing.T) {
	w, dir := newWriter(t)
	ts := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	rec := allowRecord(ts)
	rec.SQL = strings.Repeat("x", 8<<10)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			if err := w.Append(rec); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := len(readLines(t, filepath.Join(dir, "audit-2026-11-25.jsonl"))); n != 50 {
		t.Fatalf("lines = %d; want 50", n)
	}
}

func TestFailedWriteLeavesNoPartialLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("RLIMIT_FSIZE does not apply")
	}
	w, dir := newWriter(t)
	ts := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	if err := w.Append(allowRecord(ts)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "audit-2026-11-25.jsonl")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	signal.Ignore(syscall.SIGXFSZ)
	defer signal.Reset(syscall.SIGXFSZ)
	var old syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Fatal(err)
	}
	limit := old
	limit.Cur = uint64(len(before) + 100)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	big := allowRecord(ts)
	big.SQL = strings.Repeat("x", 8<<10)
	appendErr := w.Append(big)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &old); err != nil {
		t.Fatal(err)
	}
	if appendErr == nil {
		t.Fatal("Append above RLIMIT_FSIZE: want error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("file has %d bytes after the failed write; want the %d bytes before it", len(after), len(before))
	}
	if err := w.Append(allowRecord(ts)); err != nil {
		t.Fatal(err)
	}
	if n := len(readLines(t, path)); n != 2 {
		t.Fatalf("file has %d lines; want 2", n)
	}
}

func TestAppendDoesNotFollowSymlink(t *testing.T) {
	w, dir := newWriter(t)
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "audit-2026-11-25.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(allowRecord(time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC))); err == nil {
		t.Fatal("Append through a symbolic link: want error")
	}
	if data, err := os.ReadFile(target); err != nil || len(data) != 0 {
		t.Fatalf("link target holds %d bytes, %v; want 0", len(data), err)
	}
}
