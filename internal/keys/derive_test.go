package keys

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func testMaster() []byte {
	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i)
	}
	return master
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestJWTKeyInfoLabel(t *testing.T) {
	want := map[int]string{
		1: "ce6ad1e7bf7070ff69221e756fc18efdbb43adfa3341a63876d7528bc335a7be",
		2: "a07bdabf4e577404df9923bc2c2fbaeb9c29e51c07bab611ab8b7efb43eb552f",
	}
	for version, hexKey := range want {
		got, err := JWTKey(testMaster(), version)
		if err != nil {
			t.Fatalf("version %d: %v", version, err)
		}
		if !bytes.Equal(got, mustHex(t, hexKey)) {
			t.Fatalf("version %d: key %x, want %s", version, got, hexKey)
		}
	}
}

func TestApprovalKeyInfoLabel(t *testing.T) {
	got, err := ApprovalKey(testMaster())
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, "a21261f0469c1a425db4ac7cec299746018dde50ef6ceb9a3c4ad157b1d654f9")
	if !bytes.Equal(got, want) {
		t.Fatalf("key %x, want %x", got, want)
	}
}

func TestVersionChangesKey(t *testing.T) {
	v1, err := JWTKey(testMaster(), 1)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := JWTKey(testMaster(), 2)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := ApprovalKey(testMaster())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(v1, v2) || bytes.Equal(v1, approval) || bytes.Equal(v2, approval) {
		t.Fatal("derived keys are not distinct")
	}
	for _, k := range [][]byte{v1, v2, approval} {
		if len(k) != 32 {
			t.Fatalf("key length %d, want 32", len(k))
		}
	}
}

func TestRejectsBadMaster(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33, 64} {
		master := bytes.Repeat([]byte{7}, n)
		if _, err := JWTKey(master, 1); err == nil {
			t.Fatalf("JWTKey accepted a %d-byte master key", n)
		}
		if _, err := ApprovalKey(master); err == nil {
			t.Fatalf("ApprovalKey accepted a %d-byte master key", n)
		}
	}
}

func TestRejectsNonPositiveVersion(t *testing.T) {
	for _, v := range []int{0, -1} {
		if _, err := JWTKey(testMaster(), v); err == nil {
			t.Fatalf("JWTKey accepted version %d", v)
		}
	}
}
