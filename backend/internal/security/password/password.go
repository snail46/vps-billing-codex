package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memory      = 19 * 1024
	iterations  = 2
	parallelism = 1
	saltLength  = 16
	keyLength   = 32
	MinLength   = 12
	MaxLength   = 128
)

var ErrInvalidPassword = errors.New("password must contain between 12 and 128 characters")

func Hash(value string) (string, error) {
	if len([]rune(value)) < MinLength || len([]rune(value)) > MaxLength {
		return "", ErrInvalidPassword
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	digest := argon2.IDKey([]byte(value), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(digest)), nil
}

func Verify(encoded, value string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var parsedMemory uint32
	var parsedIterations uint32
	var parsedParallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parsedMemory, &parsedIterations, &parsedParallelism); err != nil {
		return false
	}
	if parsedMemory != memory || parsedIterations != iterations || parsedParallelism != parallelism {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != keyLength {
		return false
	}
	got := argon2.IDKey([]byte(value), salt, parsedIterations, parsedMemory, parsedParallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
