package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const keySize = 32

type Cipher struct {
	aead cipher.AEAD
}

func LoadOrCreateCipher(dataDir string, encryptedTokenCount int) (*Cipher, error) {
	path := filepath.Join(dataDir, "master.key")
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if encryptedTokenCount > 0 {
			return nil, fmt.Errorf("master key is missing while encrypted API tokens exist")
		}
		key = make([]byte, keySize)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("generate master key: %w", err)
		}
		if err := os.WriteFile(path, []byte(base64.RawStdEncoding.EncodeToString(key)), 0o600); err != nil {
			return nil, fmt.Errorf("persist master key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	} else {
		decoded, decodeErr := base64.RawStdEncoding.DecodeString(string(key))
		if decodeErr != nil {
			return nil, fmt.Errorf("decode master key: %w", decodeErr)
		}
		key = decoded
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("master key must be %d bytes", keySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

func (c *Cipher) Encrypt(plaintext string) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, c.aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return c.aead.Seal(nil, nonce, []byte(plaintext), nil), nonce, nil
}

func (c *Cipher) Decrypt(ciphertext, nonce []byte) (string, error) {
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt API token: %w", err)
	}
	return string(plaintext), nil
}
