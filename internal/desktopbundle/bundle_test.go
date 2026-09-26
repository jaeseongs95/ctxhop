package desktopbundle

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CCCCY-ci/ctxhop/internal/crypto"
	"github.com/CCCCY-ci/ctxhop/internal/remote"
)

func fixture(t *testing.T) (*remote.Dir, *ecdh.PrivateKey, string, Metadata) {
	t.Helper()
	root := t.TempDir()
	store, err := remote.NewDir(root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := Metadata{SessionID: "12345678-1234-1234-1234-123456789abc", Title: "private-title-비공개", SourceCwd: `C:\private-work`, UpdatedAt: "2026-09-26T00:00:00Z", HistoryMode: "paginated", CLIVersion: "0.1.0", RecordCount: 3}
	return store, identity, root, m
}

func publish(t *testing.T, store *remote.Dir, identity *ecdh.PrivateKey, root string, m Metadata, body []byte) Result {
	t.Helper()
	input := filepath.Join(t.TempDir(), "input.zip")
	if err := os.WriteFile(input, body, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Put(context.Background(), store, identity.PublicKey(), "device1", input, m)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMultiChunkRoundtripEncryptedAndImmutable(t *testing.T) {
	store, identity, root, m := fixture(t)
	body := bytes.Repeat([]byte("private-body-秘密\n"), (ChunkBytes*2)/len("private-body-秘密\n")+101)
	result := publish(t, store, identity, root, m, body)
	items, err := List(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, nil)
	if err != nil || len(items) != 1 || items[0].Metadata != m {
		t.Fatalf("list: %v %v", items, err)
	}
	output := filepath.Join(t.TempDir(), "output.zip")
	got, err := Get(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, result.ID, output)
	if err != nil || got != result {
		t.Fatalf("get: %v %v", got, err)
	}
	actual, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(actual, body) {
		t.Fatal("roundtrip content mismatch")
	}
	objects, err := store.List(context.Background(), Prefix)
	if err != nil || len(objects) != 4 {
		t.Fatalf("chunks: %d %v", len(objects), err)
	}
	for _, object := range objects {
		sealed, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(object.Key)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(sealed, []byte("ASx1")) || bytes.Contains(sealed, []byte(m.Title)) || bytes.Contains(sealed, []byte(m.SourceCwd)) || bytes.Contains(sealed, []byte("private-body")) {
			t.Fatal("plaintext leaked")
		}
	}
	if err := putEncrypted(context.Background(), store, identity.PublicKey(), Prefix+result.ID+"/metadata", []byte("replacement")); err == nil {
		t.Fatal("immutable metadata replaced")
	}
	if _, err := Get(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, result.ID, output); err == nil {
		t.Fatal("existing output accepted")
	}
	actual, _ = os.ReadFile(output)
	if !bytes.Equal(actual, body) {
		t.Fatal("existing output changed")
	}
}

func TestCorruptionWrongKeyChunkSwapAndNoPartialOutput(t *testing.T) {
	for _, attack := range []string{"wrongkey", "ciphertext", "chunk-swap", "whole-digest", "chunk-digest", "traversal", "huge-envelope", "null-metadata-field"} {
		t.Run(attack, func(t *testing.T) {
			store, identity, root, m := fixture(t)
			result := publish(t, store, identity, root, m, bytes.Repeat([]byte("content"), ChunkBytes/7+10))
			keys := []*ecdh.PrivateKey{identity}
			metadataKey := Prefix + result.ID + "/metadata"
			man, err := readManifest(context.Background(), store, keys, result.ID)
			if err != nil {
				t.Fatal(err)
			}
			chunkKey := Prefix + result.ID + "/chunk-000"
			switch attack {
			case "wrongkey":
				other, _ := ecdh.X25519().GenerateKey(rand.Reader)
				keys = []*ecdh.PrivateKey{other}
			case "ciphertext":
				file := filepath.Join(root, filepath.FromSlash(chunkKey))
				sealed, _ := os.ReadFile(file)
				sealed[len(sealed)-1] ^= 1
				os.WriteFile(file, sealed, 0600)
			case "chunk-swap":
				sealed, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(Prefix+result.ID+"/chunk-001")))
				os.WriteFile(filepath.Join(root, filepath.FromSlash(chunkKey)), sealed, 0600)
			case "whole-digest":
				man.SHA256 = strings.Repeat("0", 64)
			case "chunk-digest":
				man.Chunks[0].SHA256 = strings.Repeat("0", 64)
			case "traversal":
				man.Chunks[0].Name = "../../outside"
			case "huge-envelope":
				os.WriteFile(filepath.Join(root, filepath.FromSlash(metadataKey)), make([]byte, maxManifestBytes+sealOverhead+1), 0600)
			case "null-metadata-field":
				data, _ := json.Marshal(man)
				data = bytes.Replace(data, []byte(`"recordCount":3`), []byte(`"recordCount":null`), 1)
				sealed, _ := crypto.Encrypt(identity.PublicKey(), metadataKey, data)
				os.WriteFile(filepath.Join(root, filepath.FromSlash(metadataKey)), sealed, 0600)
			}
			if attack == "whole-digest" || attack == "chunk-digest" || attack == "traversal" {
				data, _ := json.Marshal(man)
				sealed, _ := crypto.Encrypt(identity.PublicKey(), metadataKey, data)
				os.WriteFile(filepath.Join(root, filepath.FromSlash(metadataKey)), sealed, 0600)
			}
			outputRoot := t.TempDir()
			output := filepath.Join(outputRoot, "result.zip")
			if _, err := Get(context.Background(), store, keys, nil, result.ID, output); err == nil {
				t.Fatal("attack accepted")
			}
			entries, err := os.ReadDir(outputRoot)
			if err != nil || len(entries) != 0 {
				t.Fatalf("partial files remain: %v %v", entries, err)
			}
			if attack == "wrongkey" || attack == "huge-envelope" || attack == "null-metadata-field" {
				var diagnostic bytes.Buffer
				items, err := List(context.Background(), store, keys, nil, &diagnostic)
				if err != nil || len(items) != 0 || diagnostic.Len() == 0 || strings.Contains(diagnostic.String(), m.Title) || strings.Contains(diagnostic.String(), m.SourceCwd) {
					t.Fatalf("unsafe list diagnostic: %v %v", items, err)
				}
			}
		})
	}
}

func TestMetadataAndIDHostileInputs(t *testing.T) {
	_, _, _, m := fixture(t)
	good, _ := json.Marshal(m)
	if _, err := DecodeMetadata(good); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		append(append([]byte{}, good[:len(good)-1]...), []byte(`,"unexpected":"private"}`)...),
		append(append([]byte{}, good[:len(good)-1]...), []byte(`,"title":"duplicate"}`)...),
		bytes.Replace(good, []byte(`"recordCount":3`), []byte(`"recordCount":null`), 1),
		bytes.Replace(good, []byte(`"historyMode":"paginated"`), []byte(`"historyMode":""`), 1),
		bytes.Replace(good, []byte(`"title"`), []byte(`"Title"`), 1),
		append(good, []byte(" {}")...),
		bytes.Repeat([]byte("x"), maxMetadataBytes+1),
	} {
		if _, err := DecodeMetadata(data); err == nil {
			t.Fatal("hostile metadata accepted")
		}
	}
	for _, id := range []string{"../outside", "device1/../outside", `device1\abcd`, "device1/" + strings.Repeat("A", 32), "device1/" + strings.Repeat("a", 33), "C:/" + strings.Repeat("a", 32), "device1/" + strings.Repeat("a", 32) + "/child"} {
		if ValidateID(id) == nil {
			t.Fatalf("hostile ID accepted: %q", id)
		}
	}
}

func TestOversizeInputRefusedBeforePublishing(t *testing.T) {
	store, identity, _, m := fixture(t)
	input := filepath.Join(t.TempDir(), "huge.zip")
	f, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := Put(context.Background(), store, identity.PublicKey(), "device1", input, m); err == nil {
		t.Fatal("oversize input accepted")
	}
	objects, _ := store.List(context.Background(), Prefix)
	if len(objects) != 0 {
		t.Fatal("oversize input published objects")
	}
}

func TestMembershipAndEmptyArchive(t *testing.T) {
	store, identity, root, m := fixture(t)
	result := publish(t, store, identity, root, m, nil)
	items, err := List(context.Background(), store, []*ecdh.PrivateKey{identity}, map[string]struct{}{"other": {}}, nil)
	if err != nil || len(items) != 0 {
		t.Fatal("unauthorized source listed")
	}
	if _, err := Get(context.Background(), store, []*ecdh.PrivateKey{identity}, map[string]struct{}{"other": {}}, result.ID, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("unauthorized source downloaded")
	}
	if _, err := Get(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, result.ID, filepath.Join(t.TempDir(), "empty")); err != nil {
		t.Fatal(err)
	}
}

type failUpload struct {
	remote.Remote
	puts []string
}

func (s *failUpload) Put(ctx context.Context, key string, body io.Reader, size int64) error {
	s.puts = append(s.puts, key)
	if strings.HasSuffix(key, "/chunk-001") {
		return errors.New("isolated injected failure")
	}
	return s.Remote.Put(ctx, key, body, size)
}

func TestFailedUploadDoesNotPublishMetadataOrDeleteAdjacentData(t *testing.T) {
	store, identity, _, m := fixture(t)
	ctx := context.Background()
	store.Put(ctx, "adjacent", strings.NewReader("keep"), 4)
	input := filepath.Join(t.TempDir(), "input")
	os.WriteFile(input, make([]byte, ChunkBytes+1), 0600)
	failure := &failUpload{Remote: store}
	if _, err := Put(ctx, failure, identity.PublicKey(), "device1", input, m); err == nil {
		t.Fatal("injected upload failure ignored")
	}
	if len(failure.puts) != 2 || !strings.HasSuffix(failure.puts[0], "/chunk-000") || !strings.HasSuffix(failure.puts[1], "/chunk-001") {
		t.Fatal("unexpected publication order")
	}
	items, err := List(ctx, store, []*ecdh.PrivateKey{identity}, nil, nil)
	if err != nil || len(items) != 0 {
		t.Fatal("partial upload listed")
	}
	if _, err := store.Stat(ctx, "adjacent"); err != nil {
		t.Fatal("adjacent data modified")
	}
}

func TestConcurrentImmutableSnapshots(t *testing.T) {
	store, identity, _, m := fixture(t)
	input := filepath.Join(t.TempDir(), "input")
	os.WriteFile(input, []byte("snapshot"), 0600)
	var workers sync.WaitGroup
	results := make(chan Result, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := Put(context.Background(), store, identity.PublicKey(), "device1", input, m)
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}()
	}
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for result := range results {
		if seen[result.ID] {
			t.Fatal("snapshot collision")
		}
		seen[result.ID] = true
	}
	items, err := List(context.Background(), store, []*ecdh.PrivateKey{identity}, nil, nil)
	if err != nil || len(items) != 8 {
		t.Fatalf("concurrent snapshot count: %d %v", len(items), err)
	}
}
