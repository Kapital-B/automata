package security

import (
	"errors"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = domainauth.ErrInvalidToken

type Claims struct {
	UserID uuid.UUID `json:"uid"`
	jwt.RegisteredClaims
}

func SignJWT(secret []byte, userID uuid.UUID, ttl time.Duration) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("jwt secret too short")
	}
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(secret)
}

func ParseJWT(secret []byte, tokenStr string) (uuid.UUID, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}
	c, ok := t.Claims.(*Claims)
	if !ok || !t.Valid {
		return uuid.Nil, ErrInvalidToken
	}
	return c.UserID, nil
}

// JWT implements the access-token ports with the existing signing and parsing behavior.
type JWT struct {
	Secret []byte
	TTL    time.Duration
}

func (j JWT) Issue(userID uuid.UUID) (string, error) { return SignJWT(j.Secret, userID, j.TTL) }
func (j JWT) Verify(token string) (uuid.UUID, error) { return ParseJWT(j.Secret, token) }

var _ domainauth.AccessTokenIssuer = JWT{}
var _ domainauth.AccessTokenVerifier = JWT{}
