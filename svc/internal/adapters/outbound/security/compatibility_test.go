package security

import (
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"
)

func TestAccessTokenPortCompatibility(t *testing.T) {
	secret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	id := uuid.New()
	adapter := JWT{Secret: secret, TTL: time.Hour}
	old, err := SignJWT(secret, id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Verify(old)
	if err != nil || got != id {
		t.Fatalf("verify existing token: %v %v", got, err)
	}
	token, err := adapter.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err = ParseJWT(secret, token)
	if err != nil || got != id {
		t.Fatalf("parse new token: %v %v", got, err)
	}
	expired, err := SignJWT(secret, id, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.Verify(expired); err == nil {
		t.Fatal("expired token accepted")
	}
}
func TestPasswordPortCompatibility(t *testing.T) {
	adapter := BcryptHasher{}
	old, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if err = adapter.Verify(string(old), "password123"); err != nil {
		t.Fatal(err)
	}
	if err = adapter.Verify(string(old), "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	hash, err := adapter.Hash("password123")
	if err != nil {
		t.Fatal(err)
	}
	if err = bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123")); err != nil {
		t.Fatal(err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != bcrypt.DefaultCost {
		t.Fatalf("hash cost = %d %v", cost, err)
	}
}
