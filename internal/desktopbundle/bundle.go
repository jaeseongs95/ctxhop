// Package desktopbundle transfers opaque Desktop archives through the existing
// encrypted CtxHop domain. It never interprets or installs archive contents.
package desktopbundle

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CCCCY-ci/ctxhop/internal/config"
	"github.com/CCCCY-ci/ctxhop/internal/crypto"
	"github.com/CCCCY-ci/ctxhop/internal/remote"
)

const (
	Prefix           = "v1/desktop-bundles/"
	ChunkBytes       = 8 << 20
	MaxBytes         = 1 << 30
	maxMetadataBytes = 64 << 10
	maxManifestBytes = 128 << 10
	sealOverhead     = 65 // existing crypto object header + GCM tag
	maxListedObjects = 100000
)

type Metadata struct {
	SessionID   string `json:"sessionId"`
	Title       string `json:"title"`
	SourceCwd   string `json:"sourceCwd"`
	UpdatedAt   string `json:"updatedAt"`
	HistoryMode string `json:"historyMode"`
	CLIVersion  string `json:"cliVersion"`
	RecordCount uint64 `json:"recordCount"`
}

type Result struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Info struct {
	Result
	Metadata Metadata `json:"metadata"`
}

type chunk struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type manifest struct {
	Version  int      `json:"version"`
	ID       string   `json:"id"`
	Metadata Metadata `json:"metadata"`
	Bytes    int64    `json:"bytes"`
	SHA256   string   `json:"sha256"`
	Chunks   []chunk  `json:"chunks"`
}

// DecodeMetadata accepts exactly the whitelist, including all seven fields.
func DecodeMetadata(data []byte) (Metadata, error) {
	var m Metadata
	if len(data) > maxMetadataBytes || strictJSON(data, &m) != nil {
		return m, errors.New("bundle: invalid metadata JSON")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 7 {
		return m, errors.New("bundle: metadata requires all seven whitelist fields")
	}
	for _, key := range []string{"sessionId", "title", "sourceCwd", "updatedAt", "historyMode", "cliVersion", "recordCount"} {
		if _, ok := fields[key]; !ok {
			return m, errors.New("bundle: metadata field name is not whitelisted")
		}
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return m, errors.New("bundle: null metadata field")
		}
	}
	return m, validateMetadata(m)
}

func ReadMetadataFile(file string) (Metadata, error) {
	root, name, err := localRoot(file)
	if err != nil {
		return Metadata{}, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return Metadata{}, errors.New("bundle: metadata file is unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Metadata{}, errors.New("bundle: metadata must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxMetadataBytes+1))
	if err != nil {
		return Metadata{}, errors.New("bundle: metadata file read failed")
	}
	return DecodeMetadata(data)
}

func validateMetadata(m Metadata) error {
	if len(m.SessionID) != 36 {
		return errors.New("bundle: invalid session UUID")
	}
	for i, ch := range m.SessionID {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if ch != '-' {
				return errors.New("bundle: invalid session UUID")
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", ch) {
			return errors.New("bundle: invalid session UUID")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, m.UpdatedAt); err != nil {
		return errors.New("bundle: updatedAt must be RFC3339")
	}
	for _, field := range []struct {
		value string
		max   int
	}{
		{m.Title, 4096}, {m.SourceCwd, 32768}, {m.HistoryMode, 128}, {m.CLIVersion, 256},
	} {
		if len(field.value) > field.max || !utf8.ValidString(field.value) || strings.ContainsAny(field.value, "\x00\r\n") {
			return errors.New("bundle: invalid metadata string")
		}
	}
	if strings.TrimSpace(m.HistoryMode) == "" || strings.TrimSpace(m.CLIVersion) == "" {
		return errors.New("bundle: historyMode and cliVersion are required")
	}
	if m.RecordCount > 1000000000 {
		return errors.New("bundle: record count exceeds limit")
	}
	return nil
}

// ValidateID refuses anything except one device and a fresh opaque snapshot.
func ValidateID(id string) error {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || config.ValidateDeviceID(parts[0]) != nil || len(parts[1]) != 32 || !lowerHex(parts[1]) {
		return errors.New("bundle: invalid bundle ID")
	}
	return remote.ValidateKey(Prefix + id + "/metadata")
}

func lowerHex(value string) bool {
	for _, ch := range value {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			return false
		}
	}
	return value != ""
}

// Put publishes encrypted chunks first and encrypted metadata last. A failed
// upload can leave only its own unlisted ciphertext; it never deletes objects.
func Put(ctx context.Context, store remote.Remote, public *ecdh.PublicKey, device, input string, m Metadata) (Result, error) {
	if ctx == nil || store == nil || public == nil {
		return Result{}, errors.New("bundle: storage and encryption context are required")
	}
	if err := config.ValidateDeviceID(device); err != nil {
		return Result{}, errors.New("bundle: invalid local device ID")
	}
	if err := validateMetadata(m); err != nil {
		return Result{}, err
	}
	root, name, err := localRoot(input)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return Result{}, errors.New("bundle: input is unavailable")
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return Result{}, errors.New("bundle: input must be a regular file")
	}
	if stat.Size() > MaxBytes {
		return Result{}, errors.New("bundle: input exceeds 1 GiB")
	}
	if err := checkDirectoryStore(store); err != nil {
		return Result{}, err
	}
	entropy := make([]byte, 16)
	rand.Read(entropy)
	id := device + "/" + hex.EncodeToString(entropy)
	man := manifest{Version: 1, ID: id, Metadata: m, Chunks: make([]chunk, 0)}
	hash := sha256.New()
	buffer := make([]byte, ChunkBytes)
	for index := 0; ; index++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		n, readErr := io.ReadFull(f, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return Result{}, errors.New("bundle: input read failed")
		}
		if n == 0 {
			break
		}
		if man.Bytes+int64(n) > MaxBytes {
			return Result{}, errors.New("bundle: input exceeds 1 GiB")
		}
		body := buffer[:n]
		name := fmt.Sprintf("chunk-%03d", index)
		key := Prefix + id + "/" + name
		if err := putEncrypted(ctx, store, public, key, body); err != nil {
			return Result{}, err
		}
		hash.Write(body)
		digest := sha256.Sum256(body)
		man.Chunks = append(man.Chunks, chunk{Name: name, Bytes: int64(n), SHA256: hex.EncodeToString(digest[:])})
		man.Bytes += int64(n)
		if readErr != nil {
			break
		}
	}
	if man.Bytes != stat.Size() {
		return Result{}, errors.New("bundle: input changed during upload")
	}
	man.SHA256 = hex.EncodeToString(hash.Sum(nil))
	encoded, err := json.Marshal(man)
	if err != nil || len(encoded) > maxManifestBytes {
		return Result{}, errors.New("bundle: manifest exceeds limit")
	}
	if err := putEncrypted(ctx, store, public, Prefix+id+"/metadata", encoded); err != nil {
		return Result{}, err
	}
	return Result{ID: id, SHA256: man.SHA256, Bytes: man.Bytes}, nil
}

// List decrypts only bounded metadata. Rejected entries never expose decrypted
// fields to diagnostics; plaintext is returned solely in the requested JSON.
func List(ctx context.Context, store remote.Remote, identities []*ecdh.PrivateKey, allowed map[string]struct{}, diagnostic io.Writer) ([]Info, error) {
	if ctx == nil || store == nil {
		return nil, errors.New("bundle: storage context is required")
	}
	if err := checkDirectoryStore(store); err != nil {
		return nil, err
	}
	objects, err := store.List(ctx, Prefix)
	if err != nil {
		return nil, errors.New("bundle: metadata listing failed")
	}
	if len(objects) > maxListedObjects {
		return nil, errors.New("bundle: listing exceeds object limit")
	}
	items := make([]Info, 0)
	seen := make(map[string]bool)
	for _, object := range objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(object.Key, Prefix) || !strings.HasSuffix(object.Key, "/metadata") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(object.Key, Prefix), "/metadata")
		if ValidateID(id) != nil || seen[id] {
			skipDiagnostic(diagnostic)
			continue
		}
		seen[id] = true
		if !allowedID(id, allowed) {
			continue
		}
		man, err := readManifest(ctx, store, identities, id)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			skipDiagnostic(diagnostic)
			continue
		}
		items = append(items, Info{Result: Result{ID: id, SHA256: man.SHA256, Bytes: man.Bytes}, Metadata: man.Metadata})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func skipDiagnostic(w io.Writer) {
	if w != nil {
		fmt.Fprintln(w, "bundle: skipped unreadable or invalid encrypted metadata")
	}
}

func allowedID(id string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	device := strings.Split(id, "/")[0]
	_, ok := allowed[device]
	return ok
}

// Get verifies every chunk and the complete digest in a private temporary
// file, then creates the destination with O_EXCL. Existing files are untouched.
func Get(ctx context.Context, store remote.Remote, identities []*ecdh.PrivateKey, allowed map[string]struct{}, id, output string) (Result, error) {
	if ctx == nil || store == nil {
		return Result{}, errors.New("bundle: storage context is required")
	}
	if err := ValidateID(id); err != nil {
		return Result{}, err
	}
	if !allowedID(id, allowed) {
		return Result{}, errors.New("bundle: source device is not authorized")
	}
	root, name, err := localRoot(output)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	if _, err := root.Lstat(name); err == nil {
		return Result{}, errors.New("bundle: output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, errors.New("bundle: output is unavailable")
	}
	man, err := readManifest(ctx, store, identities, id)
	if err != nil {
		return Result{}, err
	}
	entropy := make([]byte, 16)
	rand.Read(entropy)
	tempName := ".ctxhop-bundle-" + hex.EncodeToString(entropy) + ".tmp"
	temp, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Result{}, errors.New("bundle: cannot create verification file")
	}
	defer func() { temp.Close(); root.Remove(tempName) }()
	hash := sha256.New()
	for _, part := range man.Chunks {
		key := Prefix + id + "/" + part.Name
		body, err := readEncrypted(ctx, store, identities, key, ChunkBytes)
		if err != nil {
			return Result{}, err
		}
		digest := sha256.Sum256(body)
		if int64(len(body)) != part.Bytes || hex.EncodeToString(digest[:]) != part.SHA256 {
			return Result{}, errors.New("bundle: chunk integrity mismatch")
		}
		if _, err := temp.Write(body); err != nil {
			return Result{}, errors.New("bundle: verification write failed")
		}
		hash.Write(body)
	}
	if hex.EncodeToString(hash.Sum(nil)) != man.SHA256 {
		return Result{}, errors.New("bundle: whole archive integrity mismatch")
	}
	if err := temp.Sync(); err != nil {
		return Result{}, errors.New("bundle: verification sync failed")
	}
	if err := temp.Close(); err != nil {
		return Result{}, errors.New("bundle: verification close failed")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	verified, err := root.Open(tempName)
	if err != nil {
		return Result{}, errors.New("bundle: verified input is unavailable")
	}
	defer verified.Close()
	final, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Result{}, errors.New("bundle: output already exists or cannot be created")
	}
	complete := false
	defer func() {
		final.Close()
		if !complete {
			root.Remove(name)
		}
	}()
	n, err := io.Copy(final, verified)
	if err != nil || n != man.Bytes {
		return Result{}, errors.New("bundle: output write failed")
	}
	if err := final.Sync(); err != nil {
		return Result{}, errors.New("bundle: output sync failed")
	}
	if err := final.Close(); err != nil {
		return Result{}, errors.New("bundle: output close failed")
	}
	complete = true
	return Result{ID: id, SHA256: man.SHA256, Bytes: man.Bytes}, nil
}

func readManifest(ctx context.Context, store remote.Remote, identities []*ecdh.PrivateKey, id string) (manifest, error) {
	var man manifest
	if err := ValidateID(id); err != nil {
		return man, err
	}
	data, err := readEncrypted(ctx, store, identities, Prefix+id+"/metadata", maxManifestBytes)
	if err != nil {
		return man, err
	}
	if err := strictJSON(data, &man); err != nil {
		return man, errors.New("bundle: invalid encrypted manifest")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 6 {
		return man, errors.New("bundle: invalid encrypted manifest fields")
	}
	for _, key := range []string{"version", "id", "metadata", "bytes", "sha256", "chunks"} {
		if _, ok := fields[key]; !ok {
			return man, errors.New("bundle: invalid encrypted manifest field name")
		}
	}
	if _, err := DecodeMetadata(fields["metadata"]); err != nil {
		return man, errors.New("bundle: invalid encrypted metadata")
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return man, errors.New("bundle: null manifest field")
		}
	}
	var chunkFields []map[string]json.RawMessage
	if json.Unmarshal(fields["chunks"], &chunkFields) != nil {
		return man, errors.New("bundle: invalid chunk fields")
	}
	for _, fields := range chunkFields {
		if len(fields) != 3 {
			return man, errors.New("bundle: invalid chunk fields")
		}
		for _, key := range []string{"name", "bytes", "sha256"} {
			value, ok := fields[key]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return man, errors.New("bundle: invalid chunk fields")
			}
		}
	}
	if man.Version != 1 || man.ID != id || man.Bytes < 0 || man.Bytes > MaxBytes || len(man.SHA256) != 64 || !lowerHex(man.SHA256) || validateMetadata(man.Metadata) != nil {
		return man, errors.New("bundle: invalid encrypted manifest")
	}
	expected := (man.Bytes + ChunkBytes - 1) / ChunkBytes
	if int64(len(man.Chunks)) != expected {
		return man, errors.New("bundle: invalid chunk count")
	}
	var total int64
	for index, part := range man.Chunks {
		wantSize := int64(ChunkBytes)
		if index == len(man.Chunks)-1 {
			wantSize = man.Bytes - total
		}
		if part.Name != fmt.Sprintf("chunk-%03d", index) || part.Bytes != wantSize || len(part.SHA256) != 64 || !lowerHex(part.SHA256) {
			return man, errors.New("bundle: invalid chunk manifest")
		}
		total += part.Bytes
	}
	if total != man.Bytes {
		return man, errors.New("bundle: invalid chunk total")
	}
	return man, nil
}

func putEncrypted(ctx context.Context, store remote.Remote, public *ecdh.PublicKey, key string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sealed, err := crypto.Encrypt(public, key, body)
	if err != nil {
		return errors.New("bundle: encryption failed")
	}
	if dir, ok := store.(*remote.Dir); ok {
		return putDir(ctx, dir, key, sealed)
	}
	if _, err := store.Stat(ctx, key); err == nil {
		return errors.New("bundle: immutable object already exists")
	} else if !errors.Is(err, remote.ErrNotFound) {
		return errors.New("bundle: object existence check failed")
	}
	if err := store.Put(ctx, key, bytes.NewReader(sealed), int64(len(sealed))); err != nil {
		return errors.New("bundle: encrypted upload failed")
	}
	return nil
}

func readEncrypted(ctx context.Context, store remote.Remote, identities []*ecdh.PrivateKey, key string, max int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(identities) == 0 || len(identities) > 128 {
		return nil, errors.New("bundle: authorized decryption key is required")
	}
	var reader io.ReadCloser
	var err error
	if dir, ok := store.(*remote.Dir); ok {
		reader, err = getDir(dir, key)
	} else {
		reader, err = store.Get(ctx, key)
	}
	if err != nil {
		return nil, errors.New("bundle: encrypted object read failed")
	}
	sealed, readErr := io.ReadAll(io.LimitReader(reader, max+sealOverhead+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || int64(len(sealed)) > max+sealOverhead {
		return nil, errors.New("bundle: encrypted object exceeds limit or cannot be read")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, identity := range identities {
		if identity == nil {
			continue
		}
		if body, err := crypto.Decrypt(identity, key, sealed); err == nil {
			if int64(len(body)) > max {
				return nil, errors.New("bundle: decrypted object exceeds limit")
			}
			return body, nil
		}
	}
	return nil, errors.New("bundle: encrypted object failed authentication")
}

// strictJSON rejects duplicate keys, excessive nesting, unknown fields,
// trailing documents and invalid UTF-8 before interpreting an envelope.
func strictJSON(data []byte, target any) error {
	if !utf8.Valid(data) {
		return errors.New("invalid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := walkJSON(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func walkJSON(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return errors.New("invalid JSON delimiter")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate JSON key")
			}
			seen[name] = true
		}
		if err := walkJSON(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func localRoot(file string) (*os.Root, string, error) {
	abs, err := filepath.Abs(file)
	if err != nil || strings.TrimSpace(file) == "" {
		return nil, "", errors.New("bundle: invalid file path")
	}
	if err := checkPath(abs, true); err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(filepath.Dir(abs))
	if err != nil {
		return nil, "", errors.New("bundle: file directory is unavailable")
	}
	return root, filepath.Base(abs), nil
}

func checkDirectoryStore(store remote.Remote) error {
	if dir, ok := store.(*remote.Dir); ok {
		if err := checkPath(dir.Root, false); err != nil {
			return err
		}
		return checkPath(filepath.Join(dir.Root, filepath.FromSlash(strings.TrimSuffix(Prefix, "/"))), true)
	}
	return nil
}

func checkPath(name string, missing bool) error {
	abs, err := filepath.Abs(name)
	if err != nil {
		return errors.New("bundle: invalid directory path")
	}
	// Inspect each ancestor, including Windows junctions. Root subsequently
	// confines actual I/O even if an untrusted sync process changes an ancestor.
	for current := abs; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			if !missing || !errors.Is(err, os.ErrNotExist) {
				return errors.New("bundle: path is unavailable")
			}
		} else {
			reparse, err := isReparse(current, info)
			if err != nil || reparse {
				return errors.New("bundle: symlink or reparse path is refused")
			}
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return nil
}

func dirRoot(dir *remote.Dir, key string, create bool) (*os.Root, error) {
	if remote.ValidateKey(key) != nil {
		return nil, errors.New("bundle: invalid object path")
	}
	if err := checkPath(filepath.Join(dir.Root, filepath.FromSlash(key)), true); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir.Root)
	if err != nil {
		return nil, errors.New("bundle: storage root is unavailable")
	}
	if create {
		if err := root.MkdirAll(filepath.FromSlash(path.Dir(key)), 0700); err != nil {
			root.Close()
			return nil, errors.New("bundle: cannot create object directory")
		}
	}
	return root, nil
}

func getDir(dir *remote.Dir, key string) (io.ReadCloser, error) {
	root, err := dirRoot(dir, key, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(filepath.FromSlash(key))
	if err != nil {
		return nil, errors.New("bundle: object is unavailable")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("bundle: object must be a regular file")
	}
	return f, nil
}

func putDir(ctx context.Context, dir *remote.Dir, key string, sealed []byte) error {
	root, err := dirRoot(dir, key, true)
	if err != nil {
		return err
	}
	defer root.Close()
	entropy := make([]byte, 16)
	rand.Read(entropy)
	tempKey := key + "." + hex.EncodeToString(entropy) + ".tmp"
	f, err := root.OpenFile(filepath.FromSlash(tempKey), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("bundle: encrypted staging create failed")
	}
	defer func() { f.Close(); root.Remove(filepath.FromSlash(tempKey)) }()
	if _, err := f.Write(sealed); err != nil {
		return errors.New("bundle: encrypted staging write failed")
	}
	if err := f.Sync(); err != nil {
		return errors.New("bundle: encrypted staging sync failed")
	}
	if err := f.Close(); err != nil {
		return errors.New("bundle: encrypted staging close failed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishNoReplace(root, filepath.FromSlash(tempKey), filepath.FromSlash(key)); err != nil {
		return errors.New("bundle: immutable encrypted publication failed")
	}
	return nil
}
