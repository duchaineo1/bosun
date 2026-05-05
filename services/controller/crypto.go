package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// decryptToken decrypts a value stored by the API's encryptToken.
// Supports "aes256gcm:<b64>" today; extend here to add "vault:<path>" later.
func decryptToken(stored string) (string, error) {
	switch {
	case strings.HasPrefix(stored, "aes256gcm:"):
		return decryptAESGCM(stored[len("aes256gcm:"):])
	default:
		return "", fmt.Errorf("unsupported credential format: %q", stored[:min(len(stored), 16)])
	}
}

func decryptAESGCM(b64 string) (string, error) {
	key, err := loadCredentialsKey()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

func loadCredentialsKey() ([]byte, error) {
	v := os.Getenv("CREDENTIALS_KEY")
	if v == "" {
		return nil, errors.New("CREDENTIALS_KEY env var not set")
	}
	key, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("invalid CREDENTIALS_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("CREDENTIALS_KEY must be 32 bytes (got %d)", len(key))
	}
	return key, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
