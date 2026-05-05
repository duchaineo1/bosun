package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// encryptToken encrypts plaintext using AES-256-GCM and returns "aes256gcm:<b64>".
// The prefix allows future backends (e.g. "vault:<path>") to coexist without migration.
func encryptToken(plaintext string) (string, error) {
	key, err := loadCredentialsKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
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
	return "aes256gcm:" + base64.StdEncoding.EncodeToString(sealed), nil
}

func loadCredentialsKey() ([]byte, error) {
	v := os.Getenv("CREDENTIALS_KEY")
	if v == "" {
		return nil, errors.New("CREDENTIALS_KEY env var not set — generate one with: openssl rand -base64 32")
	}
	key, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("invalid CREDENTIALS_KEY (must be base64): %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("CREDENTIALS_KEY must decode to 32 bytes (got %d)", len(key))
	}
	return key, nil
}
