package cli

import (
	"os"
	"syscall"
	"testing"
)

func TestRunAppliesProcessRules(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"jwt", "generate"}, {"nope"}} {
		t.Setenv("PGSSLNEGOTIATION", "direct")
		t.Setenv("PGCHANNELBINDING", "disable")
		std, _ := testIO()
		Run(args, std)
		for _, key := range []string{"PGSSLNEGOTIATION", "PGCHANNELBINDING"} {
			if _, ok := os.LookupEnv(key); ok {
				t.Fatalf("Run(%q) kept %s", args, key)
			}
		}
		var limit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &limit); err != nil {
			t.Fatal(err)
		}
		if limit.Cur != 0 || limit.Max != 0 {
			t.Fatalf("Run(%q) RLIMIT_CORE = %d/%d; want 0/0", args, limit.Cur, limit.Max)
		}
	}
}
