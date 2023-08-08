package testutil

import "testing"

func TestRetryFlakyTest3(t0 *testing.T) {
	i := 0
	RetryFlakyTest(t0, func(t *testing.T) {
		if i == MaxRetries-1 {
			return
		}
		i++
		t.Fatal(i)
	})
}
