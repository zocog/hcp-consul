// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package kvdataloss

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
	"github.com/hashicorp/consul/testing/deployer/sprawl"
)

// Refer to common.go for the detail of the topology

func Test_KV_Dataloss(t *testing.T) {
	t.Parallel()

	ct := NewCommonTopo(t)
	ct.Launch(t)

	var numKeysToSeed int
	var numKeysToWrite int
	var count atomic.Uint64

	numKeysToSeed = 1000
	numKeysToWrite = 1000

	sp := ct.Sprawl
	cfg := sp.Config()
	// Pre seed with 1,000,000-1,000,999
	require.NoError(t, ct.Sprawl.LoadKVDataToClusterWithCounter("dc1", numKeysToSeed, 0, count, &api.WriteOptions{}))

	go func() {
		// Keep writing from with 1,001,000-1,001,999
		require.NoError(t, ct.Sprawl.LoadKVDataToClusterWithCounter("dc1", numKeysToWrite, numKeysToSeed, count, &api.WriteOptions{}))
	}()

	t.Log("Start standard upgrade ...")
	require.NoError(t, sp.Upgrade(cfg, "dc1", sprawl.UpgradeTypeStandard, utils.TargetImages(), nil, nil))
	t.Log("Finished standard upgrade ...")

	// verify data is not lost
	go func() {
		currentCount := count.Load()

		checkKV(ct, currentCount)
	}()
	data, err := ct.Sprawl.GetKV("dc1", "key-0", &api.QueryOptions{})
	require.NoError(t, err)
	require.NotNil(t, data)

	ct.PostUpgradeValidation(t)
}

func checkKV(ct *commonTopo, currentCount uint64, seed int) {

	data, err := ct.Sprawl.GetKV("dc1", "key-0", &api.QueryOptions{})
}
