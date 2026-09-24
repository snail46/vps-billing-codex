package operationhttp

import (
	"testing"

	"github.com/google/uuid"
)

func TestEventBelongsToUser(t *testing.T) {
	userID := uuid.New()
	payload := []byte(`{"data":{"user_id":"` + userID.String() + `"}}`)
	if !eventBelongsToUser(payload, userID) {
		t.Fatal("event for the authenticated user was rejected")
	}
	if eventBelongsToUser(payload, uuid.New()) {
		t.Fatal("event for another user was exposed")
	}
	if eventBelongsToUser([]byte(`{"data":{}}`), userID) {
		t.Fatal("event without ownership was exposed")
	}
}
