package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/getdavit/davit/internal/state"
)

// EnvSetResult is returned after successfully storing an env var.
type EnvSetResult struct {
	Status string `json:"status"`
	App    string `json:"app"`
	Key    string `json:"key"`
}

// EnvGetResult is returned when reading an env var.
type EnvGetResult struct {
	Status string `json:"status"`
	App    string `json:"app"`
	Key    string `json:"key"`
	Value  string `json:"value"`
}

// EnvListResult is returned when listing env vars.
type EnvListResult struct {
	Status string     `json:"status"`
	App    string     `json:"app"`
	Vars   []EnvEntry `json:"vars"`
}

// EnvEntry is a single env var entry (key only, no value).
type EnvEntry struct {
	Key       string `json:"key"`
	UpdatedAt string `json:"updated_at"`
}

// getOrCreateEncKey returns the AES-256 encryption key for this installation,
// generating and persisting one to system_info on first call.
func (m *Manager) getOrCreateEncKey() ([]byte, error) {
	raw, err := m.db.GetSystemInfo("env_enc_key")
	if err != nil {
		return nil, fmt.Errorf("STATE_DB_ERROR: read enc key: %w", err)
	}
	if raw != "" {
		key, err := hex.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("INTERNAL_ERROR: decode enc key: %w", err)
		}
		return key, nil
	}
	// Generate a new 32-byte key (AES-256).
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("INTERNAL_ERROR: generate enc key: %w", err)
	}
	if err := m.db.SetSystemInfo("env_enc_key", hex.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("STATE_DB_ERROR: persist enc key: %w", err)
	}
	return key, nil
}

func encryptValue(key []byte, plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func decryptValue(key, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// EnvSet stores an encrypted environment variable for an app.
func (m *Manager) EnvSet(appName, key, value string) error {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	encKey, err := m.getOrCreateEncKey()
	if err != nil {
		return err
	}
	encrypted, err := encryptValue(encKey, value)
	if err != nil {
		return fmt.Errorf("INTERNAL_ERROR: encrypt: %w", err)
	}
	if err := m.db.SetEnvVar(appName, key, encrypted); err != nil {
		return fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	return nil
}

// EnvGet retrieves and decrypts an environment variable.
func (m *Manager) EnvGet(appName, key string) (string, error) {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return "", fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return "", fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	encrypted, err := m.db.GetEnvVar(appName, key)
	if err != nil {
		return "", fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if encrypted == nil {
		return "", fmt.Errorf("ENV_KEY_NOT_FOUND: %s", key)
	}

	encKey, err := m.getOrCreateEncKey()
	if err != nil {
		return "", err
	}
	return decryptValue(encKey, encrypted)
}

// EnvList returns metadata for all env vars of an app (keys only, no values).
func (m *Manager) EnvList(appName string) ([]state.EnvVar, error) {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return nil, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return nil, fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}
	return m.db.ListEnvVars(appName)
}

// EnvUnset removes an environment variable for an app.
func (m *Manager) EnvUnset(appName, key string) error {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}
	found, err := m.db.DeleteEnvVar(appName, key)
	if err != nil {
		return fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if !found {
		return fmt.Errorf("ENV_KEY_NOT_FOUND: %s", key)
	}
	return nil
}

// writeEnvFile decrypts all env vars for an app and writes them to <appDir>/.env.
// This is called before docker compose up so containers receive the current env.
func (m *Manager) writeEnvFile(appName, appDir string) error {
	encrypted, err := m.db.AllEnvVarsEncrypted(appName)
	if err != nil {
		return fmt.Errorf("STATE_DB_ERROR: read env vars: %w", err)
	}
	if len(encrypted) == 0 {
		// Nothing to write; remove stale file if present.
		_ = os.Remove(filepath.Join(appDir, ".env"))
		return nil
	}

	encKey, err := m.getOrCreateEncKey()
	if err != nil {
		return err
	}

	var sb strings.Builder
	for k, v := range encrypted {
		plain, err := decryptValue(encKey, v)
		if err != nil {
			return fmt.Errorf("INTERNAL_ERROR: decrypt %s: %w", k, err)
		}
		// Escape newlines within value using shell quoting.
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(plain)
		sb.WriteByte('\n')
	}

	envPath := filepath.Join(appDir, ".env")
	if err := os.WriteFile(envPath, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("INTERNAL_ERROR: write .env: %w", err)
	}
	return nil
}
