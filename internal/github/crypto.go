package github

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

// EncryptToken encrypts a token using AES-256-GCM with a key derived from the HMAC secret.
func EncryptToken(plaintext, hmacSecret string) (string, error) {
	key := deriveKey(hmacSecret)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptToken decrypts a token that was encrypted with EncryptToken.
func DecryptToken(cipherHex, hmacSecret string) (string, error) {
	key := deriveKey(hmacSecret)

	ciphertext, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", fmt.Errorf("decoding hex: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypting: %w", err)
	}

	return string(plaintext), nil
}

// deriveKey creates a 32-byte AES key from the HMAC secret using SHA-256.
func deriveKey(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}
