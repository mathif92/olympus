package pkg

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Cipher encrypts and decrypts secure_string parameter values using
// AES-256-GCM. The master key is derived from the KEY_ENC_MASTER (or
// PARAMDORA_MASTER_KEY) value; when none is configured a random key is used
// so the service runs out of the box, but secure values become unreadable
// after a restart.
type Cipher struct {
	key   [32]byte
	KeyID string
}

// NewCipher builds a Cipher from a raw master key string. An empty key yields
// a random one-shot key.
func NewCipher(masterKey string) (*Cipher, error) {
	c := &Cipher{KeyID: "paramdora-v1"}
	if masterKey == "" {
		if _, err := io.ReadFull(rand.Reader, c.key[:]); err != nil {
			return nil, err
		}
		return c, nil
	}
	// Accept either raw 32 bytes (hex/plain) or any passphrase: hash it.
	sum := sha256.Sum256([]byte(masterKey))
	c.key = sum
	return c, nil
}

// Encrypt seals the plaintext and returns base64 ciphertext locked to KeyID.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens base64 ciphertext produced by Encrypt.
func (c *Cipher) Decrypt(ciphertextB64 string) (string, error) {
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, raw := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, raw, nil)
	if err != nil {
		return "", errors.New("decryption failed: " + err.Error())
	}
	return string(plain), nil
}
