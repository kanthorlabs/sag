//go:build devxtest

package credential

import (
	"context"
	"strings"
	"testing"

	"github.com/kanthorlabs/sag/internal/secret"
)

func TestRollbackDeletesOwnSecretsOnly(t *testing.T) {
	dir := t.TempDir()
	writeSAGYAML(t, dir, baseYAML)

	store := newFakeStorage()
	store.entries["sag-res-other.conn"] = secret.NewCredential("other", []byte("v"))

	opts := baseOpts(t)
	opts.CfgDir = dir
	opts.Storage = store
	opts.StdinReader = strings.NewReader("postgres://user:pass@db.example.com/mydb\n")

	t.Setenv("SAG_TEST_FAIL_CRED_CREATE", "1")

	_ = Create(context.Background(), opts)

	for _, d := range store.deleted {
		if d == "sag-res-other.conn" {
			t.Errorf("rollback deleted unrelated secret %q", d)
		}
	}
	deleted := make(map[string]bool)
	for _, d := range store.deleted {
		deleted[d] = true
	}
	if !deleted["sag-res-mydb.conn"] {
		t.Errorf("rollback did not delete sag-res-mydb.conn; deleted=%v", store.deleted)
	}
}
