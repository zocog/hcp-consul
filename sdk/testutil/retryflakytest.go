package testutil

import (
	"testing"

	"github.com/hashicorp/consul/sdk/testutil/retry"
)

type Forever struct{}

func (f *Forever) Continue() bool {
	return true
}

// RetryFlakyTest retries a flaky test forever
func RetryFlakyTest(t0 *testing.T, f func(*testing.T)) {
	i := 0
	retry.RunWith(&Forever{}, t0, func(_ *retry.R) {
		t0.Run("rerun", func(t *testing.T) {
			if i > 0 {
				t.Logf("RetryFlakyTest: %d", i)
			}
			f(t)
			i++
		})
	})
}
