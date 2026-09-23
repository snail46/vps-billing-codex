package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

func NewToken() (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

func Digest(secret, token string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return mac.Sum(nil)
}

func CSRFToken(secret, token string) string {
	return base64.RawURLEncoding.EncodeToString(Digest(secret, "csrf:"+token))
}

func ValidCSRF(secret, token, candidate string) bool {
	want := CSRFToken(secret, token)
	return hmac.Equal([]byte(want), []byte(candidate))
}
