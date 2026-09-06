package adminstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// --- Encrypted provider credentials ---
//
// A user can save their own upstream keys so they do not have to send them on
// every request. Those are other people's vendor credentials, and this store
// persists to a plain JSON file, so they are sealed with AES-GCM under a key
// the deployment holds in NABUGATE_SECRET_KEY and never written in the clear.
//
// Two behaviours are deliberate and neither is a fallback:
//
//   - No secret configured: saving a provider key is refused. Writing plaintext
//     instead would be the breach this exists to prevent, and doing it silently
//     would be worse than refusing.
//   - Secret rotated: the old ciphertexts stop decrypting. That degrades one
//     stored credential at a time — the request carries on to the gateway's own
//     rung — rather than failing the gateway or returning a 500 to a caller who
//     did nothing wrong.

// ErrNoSecret is returned when a provider key is saved on a deployment that has
// no NABUGATE_SECRET_KEY.
var ErrNoSecret = errors.New("adminstore: no NABUGATE_SECRET_KEY, so provider keys cannot be stored")

// StoredKey is one saved upstream credential. Prefix is kept in the clear so a
// console can show which key is saved without ever decrypting it; Blob is the
// sealed value and must never leave this package.
type StoredKey struct {
	Prefix string `json:"prefix"`
	Blob   string `json:"blob"`
}

// DeriveSecret turns whatever string the deployment set into a 32-byte key. Any
// length of input works, which matters because the alternative is a deployment
// silently running unencrypted because its secret was 31 characters.
func DeriveSecret(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

func aead(secret []byte) (cipher.AEAD, error) {
	if len(secret) == 0 {
		return nil, ErrNoSecret
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// seal encrypts a credential. The nonce is prepended to the ciphertext so one
// opaque string is all that is stored.
func seal(secret []byte, plaintext string) (string, error) {
	gcm, err := aead(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// unseal decrypts a credential. A wrong or rotated secret fails here, and every
// caller treats that as "this stored key is unusable", not as an outage.
func unseal(secret []byte, blob string) (string, error) {
	gcm, err := aead(secret)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", fmt.Errorf("adminstore: stored key is not base64: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("adminstore: stored key is truncated")
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("adminstore: stored key does not decrypt under the current secret: %w", err)
	}
	return string(plain), nil
}

// keyPrefix is the identifying head of a credential, for a console that must
// show *which* key is saved without being able to show the key. Short keys are
// masked entirely rather than half-revealed.
func keyPrefix(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		return strings.Repeat("•", len(key))
	}
	return key[:6] + "…" + key[len(key)-2:]
}
