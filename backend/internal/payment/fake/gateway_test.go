package fake

import (
	"errors"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	gateway := New("a fake gateway secret that is long enough")
	payload := []byte(`{"event_id":"event-1"}`)
	if err := gateway.Verify(payload, gateway.Sign(payload)); err != nil {
		t.Fatal(err)
	}
	if err := gateway.Verify([]byte(`tampered`), gateway.Sign(payload)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("Verify() error = %v", err)
	}
}
