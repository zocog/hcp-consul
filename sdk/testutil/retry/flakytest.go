package retry

const FlakyMaxRetries = 3

// FlakyTest retries a flaky test forever
func FlakyTest(t Failer, f func(*R)) {
	retryer := &Counter{Count: FlakyMaxRetries}

	RunWith(retryer, t, func(r *R) {
		t.Log("iteration: ", retryer.Iteration())
		f(r)
	})
}
