package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CCCCY-ci/ctxhop/internal/config"
	"github.com/CCCCY-ci/ctxhop/internal/crypto"
	"github.com/CCCCY-ci/ctxhop/internal/desktopbundle"
	"github.com/CCCCY-ci/ctxhop/internal/remote"
	"github.com/CCCCY-ci/ctxhop/internal/syncer"
)

func TestBundleCLIIsolatedAuthorizedRoundtrip(t *testing.T) {
	configDir, remoteRoot, files := resolvedTempDir(t), resolvedTempDir(t), resolvedTempDir(t)
	t.Setenv("CTXHOP_CONFIG_DIR", configDir)
	store, err := remote.NewDir(remoteRoot)
	if err != nil {
		t.Fatal(err)
	}
	keyfile, _, err := crypto.NewKeyfile("test-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.PublishKeyfile(context.Background(), store, keyfile); err != nil {
		t.Fatal(err)
	}
	c := config.New()
	c.Remote = config.Remote{Type: "dir", Path: remoteRoot}
	c.Device.ID = "device1"
	c.IdentityPublic = keyfile.IdentityPublic
	if err := c.Save(configDir); err != nil {
		t.Fatal(err)
	}
	m := desktopbundle.Metadata{SessionID: "12345678-1234-1234-1234-123456789abc", Title: "desktop-private-title", SourceCwd: `C:\project`, UpdatedAt: "2026-09-26T00:00:00Z", HistoryMode: "paginated", CLIVersion: "1.0", RecordCount: 4}
	metadata, _ := json.Marshal(m)
	input, metadataFile, output := filepath.Join(files, "archive.zip"), filepath.Join(files, "metadata.json"), filepath.Join(files, "download.zip")
	os.WriteFile(input, []byte("desktop archive bytes"), 0600)
	os.WriteFile(metadataFile, metadata, 0600)
	var report bytes.Buffer
	if err := runBundleWithStreams([]string{"put", "--input", input, "--metadata", metadataFile, "--json"}, strings.NewReader(""), &report, io.Discard); err != nil {
		t.Fatal(err)
	}
	var put desktopbundle.Result
	if err := json.Unmarshal(report.Bytes(), &put); err != nil {
		t.Fatal(err)
	}
	report.Reset()
	if err := runBundleWithStreams([]string{"list", "--json"}, strings.NewReader("test-passphrase\n"), &report, io.Discard); err != nil {
		t.Fatal(err)
	}
	var list struct {
		Bundles []desktopbundle.Info `json:"bundles"`
	}
	if err := json.Unmarshal(report.Bytes(), &list); err != nil || len(list.Bundles) != 1 || list.Bundles[0].Metadata != m {
		t.Fatalf("list JSON: %s %v", report.String(), err)
	}
	report.Reset()
	if err := runBundleWithStreams([]string{"get", "--id", put.ID, "--output", output, "--json"}, strings.NewReader("test-passphrase\n"), &report, io.Discard); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(output)
	if string(actual) != "desktop archive bytes" {
		t.Fatal("CLI roundtrip mismatch")
	}
	report.Reset()
	if err := runBundleWithStreams([]string{"list", "--json"}, strings.NewReader("wrong-passphrase\n"), &report, io.Discard); err == nil || report.Len() != 0 {
		t.Fatal("wrong domain key accepted")
	}
	c.IdentityPublic = bytes.Repeat([]byte{42}, 32)
	if err := c.Save(configDir); err != nil {
		t.Fatal(err)
	}
	if err := runBundleWithStreams([]string{"list", "--json"}, strings.NewReader("test-passphrase\n"), &report, io.Discard); err == nil {
		t.Fatal("unauthorized identity domain accepted")
	}
}

func TestBundleOptionsRejectHostileAndIncompleteRequests(t *testing.T) {
	for _, args := range [][]string{nil, {"put"}, {"put", "--json", "--input", "x"}, {"list"}, {"list", "--json", "extra"}, {"get", "--json", "--id", "../escape", "--output", "x"}, {"list", "--json", "--input", "x"}, {"delete", "--json"}} {
		if _, err := parseBundleOptions(args); err == nil {
			t.Fatalf("accepted arguments: %v", args)
		}
	}
	var help bytes.Buffer
	if err := runHelp([]string{"bundle", "put"}); err != nil {
		t.Fatal(err)
	}
	if err := writeCommandDiscovery(&help, []string{"bundle", "get"}); err != nil || !strings.Contains(help.String(), "--output") {
		t.Fatal("bundle discovery missing")
	}
}

// resolvedTempDir resolves system links such as macOS /var -> /private/var.
// Bundle paths refuse every symlinked ancestor by design.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
