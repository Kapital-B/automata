package shared

// TokenVault encrypts refresh tokens at rest.
type TokenVault interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}
