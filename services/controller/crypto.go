package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// encryptToken encrypts plaintext using AES-256-GCM and returns "aes256gcm:<b64>".
// Mirrors the same function in the API service so the controller can store
// CRD-sourced credential values with the same encoding the API uses.
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

type gitAuth struct {
	token      string // HTTPS token (PAT or GitHub App installation token)
	sshKey     string // PEM private key for SSH deploy-key auth
	passphrase string // SSH key passphrase (empty = unencrypted key)
}

// resolveCredential decrypts stored credential data and returns a gitAuth
// ready for injection into the runner pod environment.
func resolveCredential(credType, credData string) (*gitAuth, error) {
	switch credType {
	case "pat":
		var d struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal([]byte(credData), &d); err != nil {
			return nil, fmt.Errorf("parse pat data: %w", err)
		}
		token, err := decryptToken(d.Token)
		if err != nil {
			return nil, fmt.Errorf("decrypt token: %w", err)
		}
		return &gitAuth{token: token}, nil

	case "github_app":
		var d struct {
			AppID          string `json:"app_id"`
			InstallationID string `json:"installation_id"`
			PrivateKey     string `json:"private_key"`
		}
		if err := json.Unmarshal([]byte(credData), &d); err != nil {
			return nil, fmt.Errorf("parse github_app data: %w", err)
		}
		pk, err := decryptToken(d.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt private key: %w", err)
		}
		token, err := generateGitHubAppToken(d.AppID, d.InstallationID, pk)
		if err != nil {
			return nil, fmt.Errorf("generate github app token: %w", err)
		}
		return &gitAuth{token: token}, nil

	case "ssh_key":
		var d struct {
			PrivateKey string `json:"private_key"`
			Passphrase string `json:"passphrase"`
		}
		if err := json.Unmarshal([]byte(credData), &d); err != nil {
			return nil, fmt.Errorf("parse ssh_key data: %w", err)
		}
		key, err := decryptToken(d.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt ssh key: %w", err)
		}
		auth := &gitAuth{sshKey: key}
		if d.Passphrase != "" {
			auth.passphrase, err = decryptToken(d.Passphrase)
			if err != nil {
				return nil, fmt.Errorf("decrypt passphrase: %w", err)
			}
		}
		return auth, nil

	default:
		return nil, fmt.Errorf("unknown credential type: %q", credType)
	}
}

func decryptToken(stored string) (string, error) {
	switch {
	case strings.HasPrefix(stored, "aes256gcm:"):
		return decryptAESGCM(stored[len("aes256gcm:"):])
	default:
		preview := stored
		if len(preview) > 16 {
			preview = preview[:16]
		}
		return "", fmt.Errorf("unsupported credential format: %q", preview)
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
