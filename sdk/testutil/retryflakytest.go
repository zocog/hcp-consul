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
func RetryFlakyTest(t *testing.T, f func()) {
	retry.RunWith(&Forever{}, t, func(_ *retry.R) {
		f()
	})
}
