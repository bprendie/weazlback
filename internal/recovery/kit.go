package recovery

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/argon2"
)

const SchemaVersion = 1

type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	CreatedAt     time.Time         `json:"created_at"`
	Warning       string            `json:"warning"`
	Files         map[string]string `json:"files"`
}
type Sources struct{ Vault, Config, KnownHosts string }
type Bundle struct {
	Manifest   Manifest
	Vault      []byte
	Config     []byte
	KnownHosts []byte
}
type envelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func Export(output string, sources Sources, passphrase []byte) error {
	if len(passphrase) == 0 {
		return errors.New("passphrase must not be empty")
	}
	plain, err := buildArchive(sources)
	if err != nil {
		return err
	}
	env, err := encrypt(plain, passphrase)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(output, append(encoded, '\n'))
}

func Verify(path string, passphrase []byte) (Manifest, error) {
	bundle, err := Open(path, passphrase)
	if err != nil {
		return Manifest{}, err
	}
	defer bundle.Close()
	return bundle.Manifest, nil
}

func Open(path string, passphrase []byte) (*Bundle, error) {
	var manifest Manifest
	if len(passphrase) == 0 {
		return nil, errors.New("passphrase must not be empty")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env envelope
	if json.Unmarshal(b, &env) != nil || env.Format != "weazlback-recovery" || env.Version != SchemaVersion {
		return nil, errors.New("invalid or unsupported recovery kit")
	}
	plain, err := decrypt(env, passphrase)
	if err != nil {
		return nil, errors.New("incorrect passphrase or damaged recovery kit")
	}
	defer zero(plain)
	manifest, content, err := verifyArchive(plain)
	if err != nil {
		return nil, err
	}
	return &Bundle{Manifest: manifest, Vault: content["vault.enc"], Config: content["config.json"], KnownHosts: content["known_hosts"]}, nil
}

func (b *Bundle) Close() {
	zero(b.Vault)
	zero(b.Config)
	zero(b.KnownHosts)
}

func buildArchive(sources Sources) ([]byte, error) {
	files := map[string]string{"vault.enc": sources.Vault, "config.json": sources.Config}
	if sources.KnownHosts != "" {
		if _, err := os.Stat(sources.KnownHosts); err == nil {
			files["known_hosts"] = sources.KnownHosts
		}
	}
	manifest := Manifest{SchemaVersion: SchemaVersion, CreatedAt: time.Now(), Warning: "NO RECOVERY: the vault passphrase is required and cannot be reset.", Files: map[string]string{}}
	content := map[string][]byte{}
	for name, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		sum := sha256.Sum256(b)
		manifest.Files[name] = hex.EncodeToString(sum[:])
		content[name] = b
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for _, name := range []string{"vault.enc", "config.json", "known_hosts"} {
		if b, ok := content[name]; ok {
			if err := add(archive, name, b); err != nil {
				return nil, err
			}
		}
	}
	if err := add(archive, "manifest.json", append(manifestBytes, '\n')); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func verifyArchive(data []byte) (Manifest, map[string][]byte, error) {
	var manifest Manifest
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return manifest, nil, err
	}
	content := map[string][]byte{}
	for _, file := range archive.File {
		if file.Name != filepath.Base(file.Name) {
			return manifest, nil, errors.New("recovery kit contains an unsafe path")
		}
		reader, err := file.Open()
		if err != nil {
			return manifest, nil, err
		}
		b, err := io.ReadAll(io.LimitReader(reader, 64<<20))
		reader.Close()
		if err != nil {
			return manifest, nil, err
		}
		content[file.Name] = b
	}
	if err := json.Unmarshal(content["manifest.json"], &manifest); err != nil {
		return manifest, nil, errors.New("recovery manifest missing or invalid")
	}
	if manifest.SchemaVersion != SchemaVersion {
		return manifest, nil, errors.New("unsupported recovery-kit schema")
	}
	for name, expected := range manifest.Files {
		b, ok := content[name]
		if !ok {
			return manifest, nil, fmt.Errorf("recovery file %s is missing", name)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != expected {
			return manifest, nil, fmt.Errorf("recovery file %s failed checksum", name)
		}
	}
	if _, ok := content["vault.enc"]; !ok {
		return manifest, nil, errors.New("encrypted vault is missing")
	}
	if _, ok := content["config.json"]; !ok {
		return manifest, nil, errors.New("recovery configuration is missing")
	}
	return manifest, content, nil
}

func encrypt(plain, passphrase []byte) (envelope, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return envelope{}, err
	}
	key := argon2.IDKey(passphrase, salt, 3, 64*1024, 4, 32)
	defer zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return envelope{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return envelope{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return envelope{}, err
	}
	return envelope{Format: "weazlback-recovery", Version: SchemaVersion, Salt: base64.StdEncoding.EncodeToString(salt), Nonce: base64.StdEncoding.EncodeToString(nonce), Ciphertext: base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil))}, nil
}

func decrypt(env envelope, passphrase []byte) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil || len(salt) != 16 {
		return nil, errors.New("invalid salt")
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, err
	}
	key := argon2.IDKey(passphrase, salt, 3, 64*1024, 4, 32)
	defer zero(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid nonce")
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".recovery-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func add(archive *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o600)
	header.SetModTime(time.Now())
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}
func zero(value []byte) {
	for i := range value {
		value[i] = 0
	}
}
