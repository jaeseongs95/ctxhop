package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CCCCY-ci/ctxhop/internal/adapter"
	"github.com/CCCCY-ci/ctxhop/internal/crypto"
	"github.com/CCCCY-ci/ctxhop/internal/environment"
	"github.com/CCCCY-ci/ctxhop/internal/remote"
	"github.com/CCCCY-ci/ctxhop/internal/sessionhub"
	"github.com/CCCCY-ci/ctxhop/internal/syncer"
)

// Exercise the command's real encrypted remote read, native session restore,
// and environment provider path, including a remote change after preview.
func TestSessionResumeNoEnvironmentAfterRemoteChange(t *testing.T) {
	for _, agent := range []string{"claude-code", "codex"} {
		t.Run(agent, func(t *testing.T) {
			ctx := context.Background()
			projectRoot, homeA, homeB := t.TempDir(), t.TempDir(), t.TempDir()
			configDir, remoteRoot := t.TempDir(), t.TempDir()
			t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "missing-claude"))
			t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "missing-codex"))
			t.Setenv("CTXHOP_CONFIG_DIR", configDir)
			t.Chdir(projectRoot)
			runResumeGit(t, projectRoot, "init", "-q")
			store, err := remote.NewDir(remoteRoot)
			if err != nil {
				t.Fatal(err)
			}
			cleanupResumeRemoteRoot(t, remoteRoot)
			keyfile, _, err := crypto.NewKeyfile("passphrase")
			if err != nil {
				t.Fatal(err)
			}
			if err := syncer.PublishKeyfile(ctx, store, keyfile); err != nil {
				t.Fatal(err)
			}
			dataKey, err := keyfile.UnlockWithPassphrase("passphrase")
			if err != nil {
				t.Fatal(err)
			}
			defer dataKey.Close()
			public, err := keyfile.IdentityPublicKey()
			if err != nil {
				t.Fatal(err)
			}
			identifierKey, err := dataKey.IdentifierKey()
			if err != nil {
				t.Fatal(err)
			}
			const identity = "manual:no-environment-regression"
			c := newCodexRoundTripConfig(t, configDir, remoteRoot, public, identifierKey, "devicea", projectRoot, identity)
			projectID, err := crypto.ProjectID(identifierKey, identity)
			if err != nil {
				t.Fatal(err)
			}
			const nativeID = "no-environment-native-session"
			var source, target adapter.SessionLayout
			settingsName, model, settingsFile, projectSettings := "codex-session-settings", "gpt-fixture", "config.toml", filepath.Join(projectRoot, ".codex", "config.toml")
			mcpBody := `{"command":"node","args":["fixture-server.js"]}`
			mcpFile := filepath.Join(homeB, "config.toml")
			if agent == "claude-code" {
				source, target = adapter.Layout{Home: homeA}, adapter.Layout{Home: homeB}
				settingsName, model, settingsFile = "claude-session-settings", "claude-fixture", "settings.json"
				projectSettings = filepath.Join(projectRoot, ".claude", "settings.json")
				mcpBody = `{"type":"stdio","command":"node","args":["fixture-server.js"]}`
				mcpFile = filepath.Join(homeB, ".claude.json")
				t.Setenv("CLAUDE_CONFIG_DIR", homeB)
				record, err := json.Marshal(map[string]any{"type": "user", "cwd": projectRoot, "timestamp": "2026-08-25T10:00:00Z", "message": map[string]any{"role": "user", "content": "native-fixture-marker"}})
				if err != nil {
					t.Fatal(err)
				}
				if err := source.ReplaceSession(projectRoot, nativeID, [][]byte{record}); err != nil {
					t.Fatal(err)
				}
			} else {
				source, target = adapter.CodexLayout{Home: homeA}, adapter.CodexLayout{Home: homeB}
				t.Setenv("CODEX_HOME", homeB)
				writeCodexRoundTripSession(t, source.(adapter.CodexLayout), projectRoot, nativeID, [][]byte{
					codexRoundTripRecord(t, "2026-08-25T10:00:00Z", "session_meta", map[string]any{"id": nativeID, "cwd": projectRoot, "cli_version": "0.149.0"}),
					codexRoundTripRecord(t, "2026-08-25T10:01:00Z", "event_msg", map[string]any{"text": "native-fixture-marker"}),
				})
			}
			refs, err := source.DiscoverSessions(projectRoot)
			if err != nil || len(refs) != 1 {
				t.Fatalf("source sessions = %v, %v", refs, err)
			}
			installation := adapter.Installation{DataDir: homeA, Version: "0.149.0", Compatibility: adapter.CompatFull}
			space := adapter.PathSpace{ProjectRoot: projectRoot, AgentHome: homeA}
			summary := pushDiscoveredSessionsWithOptions(ctx, c.Device.ID, identifierKey, projectID, identity, source, installation, space, store, public, newCodexRoundTripPusher(t, configDir), configDir, projectRoot, refs, pushSessionOptions{})
			if summary.Pushed != 1 || summary.Failed != 0 {
				t.Fatalf("fixture push = %+v", summary)
			}
			hubID, err := sessionhub.DeriveHubKey(identifierKey, sessionhub.DefaultHubLogicalID)
			if err != nil {
				t.Fatal(err)
			}
			v2ProjectID, err := sessionhub.DeriveProjectKey(identifierKey, hubID, identity)
			if err != nil {
				t.Fatal(err)
			}
			logicalID, err := sessionhub.DeriveNativeLogicalSessionKey(identifierKey, v2ProjectID, agent, nativeID)
			if err != nil {
				t.Fatal(err)
			}
			args := []string{"resume", logicalID, "--agent", agent, "--json", "--allow-limited", "--allow-divergent", "--no-workspace-context"}
			runResume := func(extra ...string) (resumeReport, error) {
				t.Helper()
				var output bytes.Buffer
				err := runSessionWithStreams(append(append([]string(nil), args...), extra...), strings.NewReader("passphrase\n"), &output, io.Discard)
				var report resumeReport
				if err == nil {
					err = json.Unmarshal(output.Bytes(), &report)
				}
				return report, err
			}
			cleanPreview, err := runResume("--preview")
			if err != nil || cleanPreview.Environment == nil || len(cleanPreview.Environment.Changes) != 0 {
				t.Fatalf("clean preview = %+v, %v", cleanPreview, err)
			}

			// The preview is now stale: publish applicable components without
			// changing the native records, exactly at the race boundary.
			var components []environment.ComponentContent
			var references []environment.Reference
			for _, fixture := range []struct{ kind, name, media, body string }{
				{"settings", settingsName, "application/json", `{"model":"` + model + `"}`},
				{"mcp", "fixture-server", "application/json", mcpBody},
				{"skill", "fixture-skill", "text/markdown", "# Native fixture skill\n"},
			} {
				component, err := environment.NewComponentContent(fixture.kind, fixture.name, "global", "", "portable", fixture.media, []byte(fixture.body))
				if err != nil {
					t.Fatal(err)
				}
				components = append(components, component)
				references = append(references, environment.Reference{Kind: fixture.kind, Name: fixture.name, Portability: "portable"})
			}
			legacyID, err := crypto.SessionID(identifierKey, projectID, nativeID)
			if err != nil {
				t.Fatal(err)
			}
			objectLayout, err := syncer.NewObjectLayout(projectID, legacyID, c.Device.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := syncer.PutEnvironmentManifest(ctx, store, public, objectLayout, references, components); err != nil {
				t.Fatal(err)
			}
			changedPreview, err := runResume("--preview")
			if err != nil || changedPreview.Environment == nil || len(changedPreview.Environment.Changes) != 3 {
				t.Fatalf("changed preview = %+v, %v", changedPreview, err)
			}
			for _, change := range changedPreview.Environment.Changes {
				if change.State != environment.ComponentStateMissing {
					t.Fatalf("fixture component is not applicable: %+v", change)
				}
			}
			skippedPreview, err := runResume("--preview", "--no-environment")
			if err != nil || !skippedPreview.Preview || !skippedPreview.EnvironmentSkipped {
				t.Fatalf("no-environment preview = %+v, %v", skippedPreview, err)
			}
			report, err := runResume("--no-environment")
			if err != nil || !report.EnvironmentSkipped || report.LogicalSession != logicalID {
				t.Fatalf("native no-environment resume = %+v, %v", report, err)
			}
			data, err := target.ReadSession(adapter.SessionRef{NativeID: nativeID, ProjectPath: projectRoot})
			if err != nil || !bytes.Contains(bytes.Join(data.Records, nil), []byte("native-fixture-marker")) {
				t.Fatalf("native session was not restored: %v", err)
			}
			paths := []string{filepath.Join(homeB, settingsFile), mcpFile, filepath.Join(homeB, "skills", "fixture-skill", "SKILL.md"), filepath.Join(configDir, "state", "environment-backups")}
			for _, path := range paths {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("--no-environment wrote %s: %v", path, err)
				}
			}

			// Positive control: these exact remote bodies really do reach the
			// providers and create all three environment targets without the flag.
			control, err := runResume("--replace-existing")
			if err != nil || control.EnvironmentSkipped || control.Environment == nil || control.Environment.Status != "applied" {
				t.Fatalf("environment apply control = %+v, %v", control, err)
			}
			for _, path := range paths[:3] {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("positive control did not create %s: %v", path, err)
				}
			}

			// A project override also must not block a session-only restore.
			body := []byte("model = \"different-fixture\"\n")
			if agent == "claude-code" {
				body = []byte(`{"model":"different-fixture"}`)
			}
			if err := os.MkdirAll(filepath.Dir(projectSettings), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(projectSettings, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := runResume("--replace-existing"); err == nil || !strings.Contains(err.Error(), "environment conflicts") {
				t.Fatalf("fixture did not produce an environment conflict: %v", err)
			}
			var before [][]byte
			for _, path := range append(paths[:3:3], projectSettings) {
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before = append(before, content)
			}
			conflictReport, err := runResume("--replace-existing", "--no-environment")
			if err != nil || !conflictReport.EnvironmentSkipped {
				t.Fatalf("no-environment conflict resume = %+v, %v", conflictReport, err)
			}
			for index, path := range append(paths[:3:3], projectSettings) {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before[index], after) {
					t.Fatalf("no-environment changed conflict fixture %s: %v", path, err)
				}
			}
		})
	}
}
