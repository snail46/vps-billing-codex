package totp

import (
	"testing"
	"time"
)

func TestRFC6238CompatibleCodeAndEncryption(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code := generate(secret, now)
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "000001"
	}
	if !Validate(secret, code, now) || Validate(secret, wrongCode, now) {
		t.Fatal("TOTP validation failed")
	}
	manager, err := NewManager("a sufficiently long encryption key")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := manager.Encrypt(secret)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := manager.Decrypt(ciphertext, nonce)
	if err != nil || decrypted != secret {
		t.Fatalf("Decrypt() = %q, %v", decrypted, err)
	}
}
