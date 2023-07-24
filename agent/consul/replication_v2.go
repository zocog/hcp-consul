// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"fmt"
	"sync"
	"time"

	metrics "github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	"golang.org/x/time/rate"

	"github.com/hashicorp/consul/lib/retry"
	"github.com/hashicorp/consul/logging"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type ReplicatorDelegateV2 interface {
	Replicate(ctx context.Context, remoteGenerations RemoteGenerations, logger hclog.Logger) (removeVersions RemoteGenerations, exit bool, err error)
	MetricName() string
}

type ReplicatorConfigV2 struct {
	// Name to be used in various logging
	Name string
	// The number of replication rounds per second that are allowed
	Rate int
	// The number of replication rounds that can be done in a burst
	Burst int
	// Minimum number of RPC failures to ignore before backing off
	MinFailures uint
	// Maximum wait time between failing RPCs
	MaxRetryWait time.Duration
	// Where to send our logs
	Logger hclog.Logger
	// Function to use for determining if an error should be suppressed
	SuppressErrorLog func(err error) bool

	// Delegate to perform each round of replication
	Delegate ReplicatorDelegateV2
}

type ReplicatorV2 struct {
	limiter          *rate.Limiter
	waiter           *retry.Waiter
	logger           hclog.Logger
	mu               sync.Mutex
	remoteGenerations   RemoteGenerations
	suppressErrorLog func(err error) bool
	delegate         ReplicatorDelegateV2
}

func NewReplicatorV2(config *ReplicatorConfigV2) (*ReplicatorV2, error) {
	if config == nil {
		return nil, fmt.Errorf("Cannot create the Replicator without a config")
	}
	if config.Logger == nil {
		logger := hclog.New(&hclog.LoggerOptions{})
		config.Logger = logger
	}
	limiter := rate.NewLimiter(rate.Limit(config.Rate), config.Burst)

	maxWait := config.MaxRetryWait
	if maxWait == 0 {
		maxWait = replicationDefaultMaxRetryWait
	}
	waiter := &retry.Waiter{
		MinFailures: config.MinFailures,
		MaxWait:     maxWait,
		Jitter:      retry.NewJitter(10),
	}
	return &ReplicatorV2{
		limiter:          limiter,
		waiter:           waiter,
		delegate:         config.Delegate,
		logger:           config.Logger.Named(logging.Replication).Named(config.Name),
		suppressErrorLog: config.SuppressErrorLog,
		remoteGenerations: make(RemoteGenerations),
	}, nil
}

func (r *ReplicatorV2) Run(ctx context.Context) error {
	defer r.logger.Info("stopped replication")

	for {
		// This ensures we aren't doing too many successful replication rounds - mostly useful when
		// the data within the primary datacenter is changing rapidly but we try to limit the amount
		// of resources replication into the secondary datacenter should take
		if err := r.limiter.Wait(ctx); err != nil {
			return nil
		}

		// Perform a single round of replication
		r.mu.Lock()
		remoteGenerations, exit, err := r.delegate.Replicate(ctx, r.remoteGenerations, r.logger)
		r.mu.Unlock()
		if exit {
			return nil
		}
		if err != nil {
			metrics.SetGauge([]string{"leader", "replication", r.delegate.MetricName(), "status"},
				0,
			)
			// reset the lastRemoteIndex when there is an RPC failure. This should cause a full sync to be done during
			// the next round of replication
			r.mu.Lock()
			r.remoteGenerations = make(RemoteGenerations)
			r.mu.Unlock()

			if r.suppressErrorLog == nil || !r.suppressErrorLog(err) {
				r.logger.Warn("replication error (will retry if still leader)", "error", err)
			}

			if err := r.waiter.Wait(ctx); err != nil {
				return nil
			}
			continue
		}

		metrics.SetGauge([]string{"leader", "replication", r.delegate.MetricName(), "status"},
			1,
		)
		// TODO what to do
		// metrics.SetGauge([]string{"leader", "replication", r.delegate.MetricName(), "index"},
		// 	float32(index),
		// )

		r.mu.Lock()
		r.remoteGenerations = remoteGenerations
		r.mu.Unlock()
		r.logger.Debug("replication completed through remote index", "remoteGenerations", remoteGenerations)
		r.waiter.Reset()
	}
}

type ReplicatorFuncV2 func(ctx context.Context, t *pbresource.Type, rv RemoteGenerations, logger hclog.Logger) (remoteGenerations RemoteGenerations, exit bool, err error)

type FunctionReplicatorV2 struct {
	ReplicateFn ReplicatorFuncV2
	Name        string
	Type        *pbresource.Type
}

func (r *FunctionReplicatorV2) MetricName() string {
	return r.Name
}

func (r *FunctionReplicatorV2) Replicate(ctx context.Context, remoteGenerations RemoteGenerations, logger hclog.Logger) (RemoteGenerations, bool, error) {
	return r.ReplicateFn(ctx, r.Type, remoteGenerations, logger)
}
