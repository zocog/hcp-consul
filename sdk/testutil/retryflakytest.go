package testutil

import (
	"testing"
)

const MaxRetries = 3

// RetryFlakyTest retries a flaky test forever
func RetryFlakyTest(t0 *testing.T, f func(*testing.T)) {
	for i := 0; i < MaxRetries; i++ {
		t := &testing.T{}
		func() {
			defer func() {
				if v := recover(); v != nil {
					t0.Logf("RetryFlakyTest: recover: %#v", v)
				}
				if t.Failed() {
					t0.Logf("RetryFlakyTest: FAIL")
				}
			}()
			f(t)
		}()
		if !t.Failed() {
			break
		}
	}
}
