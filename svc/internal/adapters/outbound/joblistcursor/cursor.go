package joblistcursor

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type Claims struct {
	Scope     string            `json:"scope"`
	UserID    uuid.UUID         `json:"user_id"`
	AccountID *uuid.UUID        `json:"account_id,omitempty"`
	JobType   string            `json:"job_type,omitempty"`
	StartKey  map[string]string `json:"start_key"`
}

type envelope struct {
	Version int    `json:"version"`
	Claims  Claims `json:"claims"`
	Sig     string `json:"sig"`
}

func Encode(secret []byte, claims Claims) (string, error) {
	if len(secret) == 0 {
		return "", fmt.Errorf("cursor secret required")
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	env := envelope{
		Version: 1,
		Claims:  claims,
		Sig:     sign(secret, payload),
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func Decode(secret []byte, token string) (Claims, error) {
	if len(secret) == 0 {
		return Claims{}, fmt.Errorf("cursor secret required")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Claims{}, fmt.Errorf("invalid cursor")
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Claims{}, fmt.Errorf("invalid cursor")
	}
	if env.Version != 1 {
		return Claims{}, fmt.Errorf("invalid cursor")
	}
	payload, err := json.Marshal(env.Claims)
	if err != nil {
		return Claims{}, fmt.Errorf("invalid cursor")
	}
	if !hmac.Equal([]byte(env.Sig), []byte(sign(secret, payload))) {
		return Claims{}, fmt.Errorf("invalid cursor")
	}
	return env.Claims, nil
}

func sign(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
