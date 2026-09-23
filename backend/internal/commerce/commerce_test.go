package commerce

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	db "vps-billing/backend/internal/store/sqlc"
)

func TestValidateBalancedLedger(t *testing.T) {
	transactionID := uuid.New()
	balanced := []db.CreateLedgerEntryParams{
		{TransactionID: transactionID, Direction: "debit", AmountMinor: 100, Currency: "USD"},
		{TransactionID: transactionID, Direction: "credit", AmountMinor: 100, Currency: "USD"},
	}
	if err := validateBalanced(balanced); err != nil {
		t.Fatal(err)
	}
	balanced[1].AmountMinor = 99
	if err := validateBalanced(balanced); err == nil {
		t.Fatal("unbalanced ledger was accepted")
	}
}

func TestCreateOrderValidation(t *testing.T) {
	service := NewService(nil)
	if _, err := service.CreateOrder(t.Context(), uuid.New(), uuid.New(), 0, "long-enough-key"); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("error = %v", err)
	}
	if _, err := service.CreateOrder(t.Context(), uuid.New(), uuid.New(), 1, "short"); !errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("error = %v", err)
	}
}
