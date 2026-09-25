package usage

import "testing"

func TestRatedAmountRoundsUpMinorUnits(t *testing.T) {
	if got := ratedAmount(gib/2, 101); got != 51 {
		t.Fatalf("ratedAmount() = %d, want 51", got)
	}
	if got := ratedAmount(0, 101); got != 0 {
		t.Fatalf("zero ratedAmount() = %d", got)
	}
}
