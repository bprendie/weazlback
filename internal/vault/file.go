package vault

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	formatVersion = 1
	keyBytes      = 32
	saltBytes     = 16
)

var ErrLocked = errors.New("vault is locked")

type envelope struct {
	Version    int    `json:"version"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type payload struct {
	Check   string            `json:"check"`
	Entries map[string]string `json:"entries"`
}

type File struct {
	mu             sync.Mutex
	path           string
	key            []byte
	salt           []byte
	entries        map[string]string
	failedAttempts int
	lockoutUntil   time.Time
}

func New(path string) *File { return &File{path: path} }

func Path(name string) (string, error) {
	if root := os.Getenv("WEAZLBACK_HOME"); root != "" {
		return filepath.Join(root, "vaults", name+".vault"), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "weazlback", "vaults", name+".vault"), nil
}

func (v *File) Exists() (bool, error) {
	_, err := os.Stat(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (v *File) Create(passphrase []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(passphrase) == 0 {
		return errors.New("passphrase must not be empty")
	}
	if exists, err := v.exists(); err != nil {
		return err
	} else if exists {
		return errors.New("vault already exists")
	}
	v.salt = make([]byte, saltBytes)
	if _, err := rand.Read(v.salt); err != nil {
		return err
	}
	v.key = derive(passphrase, v.salt)
	v.entries = map[string]string{}
	return v.save()
}

func (v *File) Unlock(passphrase []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.lockoutUntil) {
		return fmt.Errorf("too many failed attempts; try again in %s", time.Until(v.lockoutUntil).Round(time.Second))
	}
	if len(passphrase) == 0 {
		return errors.New("passphrase must not be empty")
	}
	b, err := os.ReadFile(v.path)
	if err != nil {
		return err
	}
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil || env.Version != formatVersion {
		return errors.New("invalid or unsupported vault")
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil || len(salt) != saltBytes {
		return errors.New("invalid vault salt")
	}
	key := derive(passphrase, salt)
	plain, err := open(key, env)
	if err != nil {
		v.recordFailure()
		return errors.New("incorrect passphrase or damaged vault")
	}
	var data payload
	if err := json.Unmarshal(plain, &data); err != nil || data.Check != "weazlback-vault" {
		return errors.New("incorrect passphrase or damaged vault")
	}
	v.key, v.salt, v.entries = key, salt, data.Entries
	v.failedAttempts, v.lockoutUntil = 0, time.Time{}
	if v.entries == nil {
		v.entries = map[string]string{}
	}
	return nil
}

func (v *File) recordFailure() {
	delays := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, time.Minute}
	v.failedAttempts++
	index := v.failedAttempts - 1
	if index >= len(delays) {
		index = len(delays) - 1
	}
	v.lockoutUntil = time.Now().Add(delays[index])
}

func (v *File) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range v.key {
		v.key[i] = 0
	}
	v.key, v.salt, v.entries = nil, nil, nil
}

func (v *File) Unlocked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.key) == keyBytes
}

func (v *File) Put(name string, value []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return ErrLocked
	}
	v.entries[name] = base64.StdEncoding.EncodeToString(value)
	return v.save()
}

func (v *File) Get(name string) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	value, ok := v.entries[name]
	if !ok {
		return nil, fmt.Errorf("vault entry %q not found", name)
	}
	return base64.StdEncoding.DecodeString(value)
}

func (v *File) Delete(names ...string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return ErrLocked
	}
	for _, name := range names {
		delete(v.entries, name)
	}
	return v.save()
}

// Encrypt seals non-vault data with the active vault key. The result is an
// authenticated, self-contained envelope and is only useful while this vault
// can be unlocked.
func (v *File) Encrypt(plain []byte) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	env, err := seal(v.key, v.salt, plain)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// Decrypt opens data produced by Encrypt with the active vault key.
func (v *File) Decrypt(encoded []byte) ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.key) == 0 {
		return nil, ErrLocked
	}
	var env envelope
	if err := json.Unmarshal(encoded, &env); err != nil || env.Version != formatVersion {
		return nil, errors.New("invalid encrypted vault payload")
	}
	return open(v.key, env)
}

func derive(passphrase, salt []byte) []byte {
	return argon2.IDKey(passphrase, salt, 3, 64*1024, 4, keyBytes)
}

func (v *File) exists() (bool, error) {
	_, err := os.Stat(v.path)
	return err == nil, errUnlessMissing(err)
}

func errUnlessMissing(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
