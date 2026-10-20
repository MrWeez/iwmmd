// Package secrets encrypts stored credentials and derives request tokens.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const versionPrefix = "v1:"

// Box encrypts and authenticates small secret values with AES-256-GCM.
type Box struct {
	aead cipher.AEAD
	key  []byte
}

// New builds a Box from a 32-byte master key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	keyCopy := append([]byte(nil), key...)
	return &Box{aead: aead, key: keyCopy}, nil
}

// Encrypt returns a versioned, URL-safe ciphertext.
func (b *Box) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nil, nonce, plaintext, nil)
	payload := append(nonce, sealed...)
	return versionPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

// Decrypt opens a value produced by Encrypt.
func (b *Box) Decrypt(value string) ([]byte, error) {
	if !strings.HasPrefix(value, versionPrefix) {
		return nil, errors.New("unsupported secret encoding")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, versionPrefix))
	if err != nil {
		return nil, fmt.Errorf("decode secret: %w", err)
	}
	nonceSize := b.aead.NonceSize()
	if len(payload) < nonceSize {
		return nil, errors.New("secret is truncated")
	}
	plaintext, err := b.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return nil, errors.New("decrypt secret: authentication failed")
	}
	return plaintext, nil
}

// HMAC returns a URL-safe HMAC-SHA256 token for value.
func (b *Box) HMAC(value string) string {
	mac := hmac.New(sha256.New, b.key)
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyHMAC checks a token against value in constant time.
func (b *Box) VerifyHMAC(value, token string) bool {
	expected, err := base64.RawURLEncoding.DecodeString(b.HMAC(value))
	if err != nil {
		return false
	}
	actual, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, actual)
}
