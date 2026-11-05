package worker

import "testing"

func TestThresholdUsesExactCents(t *testing.T) {
	for _, tc := range []struct {
		current, previous, threshold int64
		want                         bool
	}{
		{90000, 100000, 1000, true},
		{90001, 100000, 1000, false},
		{98500, 100000, 1000, false},
		{0, 100000, 1000, false},
		{90000, 0, 1000, false},
	} {
		if got := Qualifies(tc.current, tc.previous, tc.threshold); got != tc.want {
			t.Errorf("%+v got %v", tc, got)
		}
	}
	if got := percentBasisPoints(90000, 100000); got != 1000 {
		t.Fatalf("percent basis points: %d", got)
	}
}
