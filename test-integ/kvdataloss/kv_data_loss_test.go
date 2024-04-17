// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package kvdataloss

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
)

// Refer to common.go for the detail of the topology

// Questions:
// - Should we test with the versions of Consul that may have the bug?
// - Are there autopilot settings that a customer could enable to make the situation worse?

const (
	numKeysToSeed  = 1000
	numKeysToWrite = 1000
	startingPoint  = 1_000_000
)

func Test_KV_Dataloss(t *testing.T) {
	t.Parallel()

	ct := NewCommonTopo(t)
	ct.Launch(t)

	var count atomic.Uint64
	var apiClients map[string]*api.Client
	sp := ct.Sprawl

	// Get Clients for the servers.
	// We don't care which one is the current leader, just that we have a client for each
	// server for stale reads later.
	var testNode string
	{
		node, err := sp.Leader(ClusterName)
		require.NoError(t, err)
		client, err := sp.APIClientForNode(ClusterName, node.ID(), "")
		require.NoError(t, err)
		apiClients[node.Name] = client
		testNode = node.Name
	}

	nodes, err := sp.Followers(ClusterName)
	require.NoError(t, err)
	for _, node := range nodes {
		client, err := sp.APIClientForNode(ClusterName, node.ID(), "")
		require.NoError(t, err)
		apiClients[node.Name] = client
	}

	reply, err := apiClients[testNode].Operator().RaftGetConfiguration(&api.QueryOptions{})
	require.NoError(t, err)

	// leader is the Raft ID of the server that _starts_ as the leader for the test
	var leaderID string
	// follower is the Raft ID of the server that _starts_ as a follower for the test.
	// We only care about changing leadership between these two servers.
	var followerID string

	var wg sync.WaitGroup

	for _, s := range reply.Servers {
		if s.Leader {
			leaderID = s.ID
		}
		followerID = s.ID
	}

	// Pre seed with 1,000,000-1,000,999
	t.Log("Seeding KV Data ...")
	require.NoError(t, ct.Sprawl.LoadKVDataToClusterWithCounter("dc1", numKeysToSeed, 0, &count, &api.WriteOptions{}))

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Keep writing from with 1,001,000-1,001,999
		require.NoError(t, ct.Sprawl.LoadKVDataToClusterWithCounter("dc1", numKeysToWrite, numKeysToSeed, &count, &api.WriteOptions{}))
	}()

	go func() {
		testClient := apiClients[testNode]

		for {

			// Make sure the old follower is the leader
			resp, err := testClient.Operator().RaftLeaderTransfer(followerID, &api.QueryOptions{})
			if err != nil {
				t.Log("error initiating leadership transfer to original follower")
			}
			if !resp.Success {
				t.Log("failed to transfer leadership to original follower")
			}

			// Re-establish leadership to the original leader
			resp, err = testClient.Operator().RaftLeaderTransfer(leaderID, &api.QueryOptions{})
			if err != nil {
				t.Log("error initiating leadership transfer to original leader")
			}
			if !resp.Success {
				t.Log("failed to transfer leadership to original leader")
			}

			t.Log("completed one rotation of leadership swap")
		}
	}()

	// verify data is not lost
	// We spin up a goroutine for each server to make sure that the data is not lost
	// We need to make sure that we use a stale read so that each follower responds with its own data.
	for name, client := range apiClients {
		wg.Add(1)
		go func(name string, client *api.Client) {
			defer wg.Done()
			for {
				currentCount := count.Load()

				if currentCount == startingPoint+numKeysToWrite+numKeysToSeed {
					break
				}
				t.Log("Checking KV Data for ", "NodeName", name, "CurrentCount", currentCount)
				checkKV(t, client, int(currentCount))
			}
		}(name, client)
	}

	wg.Wait()
}

func checkKV(t *testing.T, client *api.Client, currentCount int) {
	for i := startingPoint; i < startingPoint+numKeysToSeed+currentCount; i++ {
		_, _, err := client.KV().Get("key-"+fmt.Sprint(i), &api.QueryOptions{})
		require.NoError(t, err, "could not validate", "key-id", i)
	}
}
