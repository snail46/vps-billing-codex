package portal

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestTicketInputValidation(t *testing.T) {
	service := NewService(nil)
	if _, err := service.CreateTicket(context.Background(), uuid.New(), "x", "normal", "message"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("short subject error=%v", err)
	}
	if _, err := service.CreateTicket(context.Background(), uuid.New(), "valid subject", "urgent", "message"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid priority error=%v", err)
	}
	if _, err := service.AddTicketMessage(context.Background(), uuid.New(), uuid.New(), " "); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty message error=%v", err)
	}
}
