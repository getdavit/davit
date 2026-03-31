// Package agent manages Ed25519 SSH keypairs for agent access.
package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// KeyPair holds the generated key material and the authorized_keys entry.
type KeyPair struct {
	Fingerprint        string
	PublicKeySSH       string // "ssh-ed25519 AAAA... label"
	PrivateKeyPEM      string // PEM-encoded OpenSSH private key
	AuthorizedKeyEntry string // full forced-command line
}

// GenerateEd25519 creates a new Ed25519 keypair labelled with label.
func GenerateEd25519(label string) (KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, fmt.Errorf("generate key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return KeyPair{}, fmt.Errorf("encode public key: %w", err)
	}

	privPEM, err := marshalPrivateKey(priv)
	if err != nil {
		return KeyPair{}, fmt.Errorf("marshal private key: %w", err)
	}

	pubLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + label
	fingerprint := ssh.FingerprintSHA256(sshPub)
	entry := authorizedKeyEntry(pubLine)

	return KeyPair{
		Fingerprint:        fingerprint,
		PublicKeySSH:       pubLine,
		PrivateKeyPEM:      privPEM,
		AuthorizedKeyEntry: entry,
	}, nil
}

// authorizedKeyEntry wraps the public key line with the forced-command
// restrictions defined in the spec (§12.2).
func authorizedKeyEntry(pubKeyLine string) string {
	return fmt.Sprintf(
		`command="/usr/local/bin/davit --json $SSH_ORIGINAL_COMMAND",no-pty,no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-user-rc %s`,
		pubKeyLine,
	)
}

// InstallPublicKey appends entry to the authorized_keys file at path.
// It is idempotent: if the entry already exists the file is not modified.
func InstallPublicKey(authorizedKeysPath, entry string) error {
	// Read existing content
	existing, err := os.ReadFile(authorizedKeysPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read authorized_keys: %w", err)
	}

	// Idempotency: check if the public key portion is already present.
	// Compare the key material (third field of the entry), not the whole line,
	// to handle cases where the forced-command prefix already exists.
	keyMaterial := extractKeyMaterial(entry)
	if keyMaterial != "" && strings.Contains(string(existing), keyMaterial) {
		return nil
	}

	f, err := os.OpenFile(authorizedKeysPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open authorized_keys: %w", err)
	}
	defer f.Close()

	line := entry
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		line = "\n" + line
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}
	return nil
}

// extractKeyMaterial returns the base64 key body from an authorized_keys line.
func extractKeyMaterial(line string) string {
	// The line may start with options (command="..."), so find the ssh-ed25519 / ssh-rsa token.
	parts := strings.Fields(line)
	for i, p := range parts {
		if strings.HasPrefix(p, "ssh-") || strings.HasPrefix(p, "ecdsa-") {
			if i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return ""
}

// marshalPrivateKey serializes an ed25519 private key into OpenSSH PEM format.
func marshalPrivateKey(key ed25519.PrivateKey) (string, error) {
	privPEM, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(privPEM)), nil
}
