package password

import (
	"errors"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(hash, "a sufficiently long password") || Verify(hash, "wrong password") {
		t.Fatal("password verification result is invalid")
	}
}

func TestHashRejectsShortPassword(t *testing.T) {
	_, err := Hash("too-short")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("Hash() error = %v", err)
	}
}
