package utils

import (
	"SocialMediaAPI/config"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
)

// EncryptToken encrypts a token using AES-256-GCM
// The encryption key is read from TOKEN_ENCRYPTION_KEY environment variable
func EncryptToken(token string) (string, error) {
	cfg := config.Load()
	key := cfg.TokenEncryptionKey
	if len(key) == 0 {
		// If no key is set, return token as-is (not recommended for production)
		// In production, this should fail or generate a key
		return token, nil
	}

	// Ensure key is 32 bytes for AES-256
	keyBytes := []byte(key)
	if len(keyBytes) < 32 {
		// Pad with zeros if shorter
		padded := make([]byte, 32)
		copy(padded, keyBytes)
		keyBytes = padded
	} else if len(keyBytes) > 32 {
		// Truncate if longer
		keyBytes = keyBytes[:32]
	}

	block, err := aes.NewCipher(keyBytes)
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

	ciphertext := gcm.Seal(nonce, nonce, []byte(token), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptToken decrypts a token encrypted with EncryptToken
func DecryptToken(encryptedToken string) (string, error) {
	cfg := config.Load()
	key := cfg.TokenEncryptionKey
	if len(key) == 0 {
		// If no key is set, assume token wasn't encrypted
		return encryptedToken, nil
	}

	// Ensure key is 32 bytes for AES-256
	keyBytes := []byte(key)
	if len(keyBytes) < 32 {
		// Pad with zeros if shorter
		padded := make([]byte, 32)
		copy(padded, keyBytes)
		keyBytes = padded
	} else if len(keyBytes) > 32 {
		// Truncate if longer
		keyBytes = keyBytes[:32]
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", err
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
