package commerce

import (
	"errors"
	"math"
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

func TestPromotionDiscountUsesMinorUnitIntegerMath(t *testing.T) {
	for _, test := range []struct {
		total, value int64
		kind         string
		want         int64
	}{
		{1001, 1000, "percent", 100}, {100, 500, "fixed", 100}, {math.MaxInt64, 10000, "percent", math.MaxInt64},
	} {
		if got := promotionDiscount(test.total, test.value, test.kind); got != test.want {
			t.Fatalf("discount(%d,%d,%s)=%d want %d", test.total, test.value, test.kind, got, test.want)
		}
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

func TestEqualJSONIgnoresStorageNormalization(t *testing.T) {
	original := []byte(`{"event_id":"event-1","amount_minor":9007199254740993,"currency":"USD"}`)
	storedJSONB := []byte(`{ "currency": "USD", "amount_minor": 9007199254740993, "event_id": "event-1" }`)
	if !equalJSON(original, storedJSONB) {
		t.Fatal("semantically equal JSON was rejected")
	}
	changedAmount := []byte(`{"event_id":"event-1","amount_minor":9007199254740992,"currency":"USD"}`)
	if equalJSON(original, changedAmount) {
		t.Fatal("different JSON numbers were treated as equal")
	}
	if equalJSON(original, append(original, []byte(` {}`)...)) {
		t.Fatal("multiple JSON values were accepted")
	}
}
