package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // #nosec G505 -- RFC 6238 interoperability requires HMAC-SHA1.
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const period = 30 * time.Second

type Manager struct{ aead cipher.AEAD }

func NewManager(encryptionKey string) (*Manager, error) {
	key := sha256.Sum256([]byte(encryptionKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Manager{aead: aead}, nil
}

func GenerateSecret() (string, error) {
	random := make([]byte, 20)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random), nil
}

func URI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	values := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + values.Encode()
}

func Validate(secret, code string, now time.Time) bool {
	if len(code) != 6 {
		return false
	}
	for offset := -1; offset <= 1; offset++ {
		if hmac.Equal([]byte(generate(secret, now.Add(time.Duration(offset)*period))), []byte(code)) {
			return true
		}
	}
	return false
}

func (m *Manager) Encrypt(secret string) ([]byte, []byte, error) {
	nonce := make([]byte, m.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return m.aead.Seal(nil, nonce, []byte(secret), nil), nonce, nil
}

func (m *Manager) Decrypt(ciphertext, nonce []byte) (string, error) {
	plaintext, err := m.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt TOTP secret: %w", err)
	}
	return string(plaintext), nil
}

func generate(secret string, now time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(now.Unix()/int64(period/time.Second)))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0f
	value := (uint32(digest[offset])&0x7f)<<24 | uint32(digest[offset+1])<<16 | uint32(digest[offset+2])<<8 | uint32(digest[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
