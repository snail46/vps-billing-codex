package infrastructure

import "testing"

func TestUtilizationScoreUsesMostConstrainedResource(t *testing.T) {
	got := utilizationScore(4, 8, 300, 1000, 90, 100)
	if got != 0.9 {
		t.Fatalf("utilizationScore() = %v, want 0.9", got)
	}
	if got := utilizationScore(0, 0, 0, 0, 0, 0); got != 0 {
		t.Fatalf("zero-capacity utilizationScore() = %v, want 0", got)
	}
}
