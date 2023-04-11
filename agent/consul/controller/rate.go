// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package controller

import (
	"math"
	"sync"
	"time"
)

// much of this is a re-implementation of:
// https://github.com/kubernetes/client-go/blob/release-1.25/util/workqueue/default_rate_limiters.go

// Limiter is an interface for a rate limiter that can limit
// the number of retries processed in the work queue.
type Limiter[RequestType comparable] interface {
	// NextRetry returns the remaining time until the queue should
	// reprocess a Request.
	NextRetry(request RequestType) time.Duration
	// Forget causes the Limiter to reset the backoff for the Request.
	Forget(request RequestType)
}

var _ Limiter[Request] = &ratelimiter[Request]{}

type ratelimiter[RequestType comparable] struct {
	failures map[RequestType]int
	base     time.Duration
	max      time.Duration
	mutex    sync.RWMutex
}

// NewRateLimiter returns a Limiter that does per-item exponential
// backoff.
func NewRateLimiter[RequestType comparable](base, max time.Duration) Limiter[RequestType] {
	return &ratelimiter[RequestType]{
		failures: make(map[RequestType]int),
		base:     base,
		max:      max,
	}
}

// NextRetry returns the remaining time until the queue should
// reprocess a Request.
func (r *ratelimiter[RequestType]) NextRetry(request RequestType) time.Duration {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	exponent := r.failures[request]
	r.failures[request] = r.failures[request] + 1

	backoff := float64(r.base.Nanoseconds()) * math.Pow(2, float64(exponent))
	// make sure we don't overflow time.Duration
	if backoff > math.MaxInt64 {
		return r.max
	}

	calculated := time.Duration(backoff)
	if calculated > r.max {
		return r.max
	}

	return calculated
}

// Forget causes the Limiter to reset the backoff for the Request.
func (r *ratelimiter[RequestType]) Forget(request RequestType) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	delete(r.failures, request)
}
