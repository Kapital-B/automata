package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// AESGCMVault implements TokenVault with AES-256-GCM (spec: encrypt tokens at rest).
type AESGCMVault struct {
	key []byte // 32 bytes
}

func NewAESGCMVault(key []byte) (*AESGCMVault, error) {
	if len(key) != 32 {
		return nil, errors.New("key must be 32 bytes")
	}
	return &AESGCMVault{key: append([]byte(nil), key...)}, nil
}

// Encrypt returns nonce||ciphertext (raw bytes); store as BLOB.
func (v *AESGCMVault) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (v *AESGCMVault) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := ciphertext[:ns], ciphertext[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

// EncryptString is a helper for tests.
func EncryptString(v *AESGCMVault, s string) (string, error) {
	b, err := v.Encrypt([]byte(s))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}
