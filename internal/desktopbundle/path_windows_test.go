//go:build windows

package desktopbundle

import (
	"context"
	"crypto/ecdh"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDirectoryJunctionAndOutputADSRejected(t *testing.T) {
	store, identity, root, m := fixture(t)
	outside := t.TempDir()
	junction := filepath.Join(root, "v1")
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, outside).CombinedOutput(); err != nil {
		t.Fatalf("create isolated junction: %v %s", err, output)
	}
	defer os.Remove(junction)
	input := filepath.Join(t.TempDir(), "input")
	os.WriteFile(input, []byte("secret"), 0600)
	if _, err := Put(context.Background(), store, identity.PublicKey(), "device1", input, m); err == nil {
		t.Fatal("junction publication accepted")
	}
	if _, err := List(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, nil); err == nil {
		t.Fatal("junction listing accepted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("junction target modified")
	}
	os.Remove(junction)
	result := publish(t, store, identity, root, m, []byte("secret"))
	if _, err := Get(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, result.ID, filepath.Join(t.TempDir(), "archive:secret")); err == nil {
		t.Fatal("Windows alternate data stream accepted")
	}
}
