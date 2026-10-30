package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"io"
)

const EncryptionVersion = 1

// AESGCM encrypts cloud credentials using a process-provided 256-bit key.
type AESGCM struct {
	aead cipher.AEAD
}

func NewAESGCM(masterKey []byte) (*AESGCM, error) {
	if len(masterKey) != 32 {
		return nil, errors.New("cloud account master key must be exactly 32 bytes")
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &AESGCM{aead: aead}, nil
}

func (a *AESGCM) Encrypt(plaintext, additionalData []byte) (nonce, ciphertext []byte, err error) {
	nonce = make([]byte, a.aead.NonceSize())
	if _, err := io.ReadFull(cryptorand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	return nonce, a.aead.Seal(nil, nonce, plaintext, additionalData), nil
}

func (a *AESGCM) Decrypt(nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != a.aead.NonceSize() {
		return nil, errors.New("invalid credential nonce")
	}
	plaintext, err := a.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, errors.New("credential decryption failed")
	}
	return plaintext, nil
}
