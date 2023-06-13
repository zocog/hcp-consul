// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package structs

import (
	"errors"
	"strings"
)

const (
	errNoLeader                   = "No cluster leader"
	errNoDCPath                   = "No path to datacenter"
	errDCNotAvailable             = "Remote DC has no server currently reachable"
	errNoServers                  = "No known Consul servers"
	errNotReadyForConsistentReads = "Not ready to serve consistent reads"
	errSegmentsNotSupported       = "Network segments are not supported in this version of Consul"
	errRPCRateExceeded            = "RPC rate limit exceeded"
	errServiceNotFound            = "Service not found: "
	errQueryNotFound              = "Query not found"
	errLeaderNotTracked           = "Raft leader not found in server lookup mapping"

	errConnectNotEnabled    = "Connect must be enabled in order to use this endpoint"
	errRateLimited          = "Rate limit reached, try again later" // Note: we depend on this error message in the gRPC ConnectCA.Sign endpoint (see: isRateLimitError).
	errNotPrimaryDatacenter = "not the primary datacenter"
	errStateReadOnly        = "CA Provider State is read-only"
)

var (
	ErrNoLeader                   = errors.New(errNoLeader)
	ErrNoDCPath                   = errors.New(errNoDCPath)
	ErrNoServers                  = errors.New(errNoServers)
	ErrNotReadyForConsistentReads = errors.New(errNotReadyForConsistentReads)
	ErrSegmentsNotSupported       = errors.New(errSegmentsNotSupported)
	ErrRPCRateExceeded            = errors.New(errRPCRateExceeded)
	ErrDCNotAvailable             = errors.New(errDCNotAvailable)
	ErrQueryNotFound              = errors.New(errQueryNotFound)
	ErrLeaderNotTracked           = errors.New(errLeaderNotTracked)

	// Err strings. net/rpc doesn't have a way to transport typed/rich errors so
	// we currently rely on sniffing the error string in a few cases where we need
	// to change client behavior. These are the canonical error strings to use.
	// Note though that client code can't use `err == consul.Err*` directly since
	// the error returned by RPC will be a plain error.errorString created by
	// net/rpc client so will not be the same _instance_ that this package
	// variable points to. Clients need to compare using `err.Error() ==
	// consul.ErrRateLimited.Error()` which is very sad. Short of replacing our
	// RPC mechanism it's hard to know how to make that much better though.
	ErrConnectNotEnabled    = errors.New(errConnectNotEnabled)
	ErrRateLimited          = errors.New(errRateLimited) // Note: we depend on this error message in the gRPC ConnectCA.Sign endpoint (see: isRateLimitError).
	ErrNotPrimaryDatacenter = errors.New(errNotPrimaryDatacenter)
	ErrStateReadOnly        = errors.New(errStateReadOnly)
)

func IsErrNoDCPath(err error) bool {
	return err != nil && strings.Contains(err.Error(), errNoDCPath)
}

func IsErrQueryNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), errQueryNotFound)
}

func IsErrNoLeader(err error) bool {
	return err != nil && strings.Contains(err.Error(), errNoLeader)
}

func IsErrRPCRateExceeded(err error) bool {
	return err != nil && strings.Contains(err.Error(), errRPCRateExceeded)
}

func IsErrServiceNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), errServiceNotFound)
}
