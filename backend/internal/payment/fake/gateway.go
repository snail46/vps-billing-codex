package fake

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var ErrInvalidSignature = errors.New("invalid payment webhook signature")

type Gateway struct{ secret []byte }

func New(secret string) *Gateway { return &Gateway{secret: []byte(secret)} }

func (g *Gateway) Sign(payload []byte) string {
	mac := hmac.New(sha256.New, g.secret)
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (g *Gateway) Verify(payload []byte, signature string) error {
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	want, _ := hex.DecodeString(g.Sign(payload))
	if !hmac.Equal(want, provided) {
		return ErrInvalidSignature
	}
	return nil
}
