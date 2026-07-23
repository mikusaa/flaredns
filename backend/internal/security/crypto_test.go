package security

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCipherPersistsAndEncrypts(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateCipher(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := first.Encrypt("cloudflare-secret-token")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("cloudflare-secret-token")) {
		t.Fatal("ciphertext contains plaintext")
	}
	second, err := LoadOrCreateCipher(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	value, err := second.Decrypt(ciphertext, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if value != "cloudflare-secret-token" {
		t.Fatalf("unexpected decrypted value %q", value)
	}
	info, err := os.Stat(filepath.Join(dir, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("master key mode is %o", info.Mode().Perm())
	}
}

func TestCipherRefusesMissingKeyForExistingTokens(t *testing.T) {
	if _, err := LoadOrCreateCipher(t.TempDir(), 1); err == nil {
		t.Fatal("expected startup refusal")
	}
}
