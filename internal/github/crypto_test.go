package github

import (
	"testing"
)

func TestEncryptDecryptToken(t *testing.T) {
	secret := "test-hmac-secret-key-12345"
	token := "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	encrypted, err := EncryptToken(token, secret)
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}

	if encrypted == token {
		t.Fatal("encrypted should differ from plaintext")
	}

	decrypted, err := DecryptToken(encrypted, secret)
	if err != nil {
		t.Fatalf("DecryptToken: %v", err)
	}

	if decrypted != token {
		t.Fatalf("expected %q, got %q", token, decrypted)
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	encrypted, err := EncryptToken("my-token", "correct-secret")
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}

	_, err = DecryptToken(encrypted, "wrong-secret")
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	secret := "my-secret"
	token := "my-token"

	enc1, _ := EncryptToken(token, secret)
	enc2, _ := EncryptToken(token, secret)

	if enc1 == enc2 {
		t.Fatal("two encryptions of the same value should produce different ciphertexts (random nonce)")
	}
}
