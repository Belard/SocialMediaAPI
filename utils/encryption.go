package utils

import (
	"SocialMediaAPI/config"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var (
	errInvalidEncryptionKeyLength = errors.New("TOKEN_ENCRYPTION_KEY must be exactly 32 bytes for AES-256")
	errCiphertextTooShort         = errors.New("encrypted token is too short or malformed")
)

// EncryptToken encrypts a token using AES-256-GCM
// The encryption key is read from TOKEN_ENCRYPTION_KEY environment variable
func EncryptToken(token string) (string, error) {
	keyBytes, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	if len(keyBytes) == 0 {
		// If no key is set, return token as-is (not recommended for production)
		return token, nil
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
	keyBytes, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	if len(keyBytes) == 0 {
		// If no key is set, assume token wasn't encrypted
		return encryptedToken, nil
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
	minSize := nonceSize + gcm.Overhead()
	if len(data) < minSize {
		return "", errCiphertextTooShort
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func getEncryptionKey() ([]byte, error) {
	cfg := config.Load()
	key := cfg.TokenEncryptionKey
	if len(key) == 0 {
		return nil, nil
	}

	keyBytes := []byte(key)
	if len(keyBytes) != 32 {
		return nil, errInvalidEncryptionKeyLength
	}

	return keyBytes, nil
}
