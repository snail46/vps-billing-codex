package admin

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestAdminInputValidation(t *testing.T) {
	service := NewService(nil)
	if _, err := service.AdjustWallet(t.Context(), uuid.New(), "US", 0, "", AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("adjust validation=%v", err)
	}
	if _, err := service.UpdateUserStatus(t.Context(), uuid.New(), "deleted", AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("status validation=%v", err)
	}
	if _, err := service.ReplyTicket(t.Context(), uuid.New(), "   ", AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reply validation=%v", err)
	}
	if _, err := service.UpdateTicketStatus(t.Context(), uuid.New(), "invalid", AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ticket validation=%v", err)
	}
}
