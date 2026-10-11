package security

import (
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	"golang.org/x/crypto/bcrypt"
)

type BcryptHasher struct{ Cost int }

func (b BcryptHasher) Hash(password string) (string, error) {
	cost := b.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(hash), err
}
func (b BcryptHasher) Verify(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

var _ domainauth.PasswordHasher = BcryptHasher{}
