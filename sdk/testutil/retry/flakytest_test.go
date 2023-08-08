package retry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlakyTestMaxMinus1(t *testing.T) {
	i := 0
	FlakyTest(t, func(r *R) {
		if i == FlakyMaxRetries-1 {
			return
		}
		i++
		r.Fatal(i)
	})
}

func TestFlakyTestMax(t *testing.T) {
	i := 0
	ft := &fakeT{}
	FlakyTest(ft, func(r *R) {
		if i == FlakyMaxRetries {
			return
		}
		i++
		r.Fatal(i)
	})
	assert.Equal(t, 1, ft.fails)
}

func TestFlakyTestPanic(t *testing.T) {
	t.Skip("TODO: not sure what the intended behavior of a panic inside a retry is; expected it to retry, but looks like it doesn't")
	i := 0
	FlakyTest(t, func(r *R) {
		if i == FlakyMaxRetries-1 {
			return
		}
		i++
		panic(i)
	})
}
