package session

import "testing"

func TestTokenAndCSRF(t *testing.T) {
	token, err := NewToken()
	if err != nil || token == "" {
		t.Fatalf("NewToken() = %q, %v", token, err)
	}
	csrf := CSRFToken("csrf-secret", token)
	if !ValidCSRF("csrf-secret", token, csrf) || ValidCSRF("csrf-secret", token, "bad") {
		t.Fatal("CSRF validation result is invalid")
	}
	if string(Digest("one", token)) == string(Digest("two", token)) {
		t.Fatal("session digest is not secret-bound")
	}
}
