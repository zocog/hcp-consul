// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package kvdataloss

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil/retry"
)

// Refer to common.go for the detail of the topology

// Questions:
// - Should we test with the versions of Consul that may have the bug?
// - Are there autopilot settings that a customer could enable to make the situation worse?

const (
	numKeysToSeed  = 1000
	numKeysToWrite = 1000
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

		// If node is server3, we need to initiate a leadership transfer to server1
		// Make sure the old follower is the leader
		if strings.Contains(node.Name, "server3") {
			t.Fatal("THE OLD BOI IS THE LEADER!")
		}
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

	for _, s := range reply.Servers {
		if s.Leader {
			leaderID = s.ID
		}
		followerID = s.ID
	}

	t.Log("Starting test...")
	count.Store(0) // reset the counter
	var wg sync.WaitGroup

	// Set up a goroutine to write KV data
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < numKeysToWrite; i++ {
			err := writeKV(t, apiClients[testNode], "test", i, &count)
			require.NoError(t, err)
		}
	}()

	// Set up a goroutine to swap leadership between the leader and follower
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		testClient := apiClients[testNode]

		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Make sure the old follower is the leader
				resp, err := testClient.Operator().RaftLeaderTransfer(followerID, &api.QueryOptions{})
				if err != nil {
					t.Logf("error initiating leadership transfer to original follower: %s", err)
				} else if !resp.Success {
					t.Log("failed to transfer leadership to original follower")
				}

				// Re-establish leadership to the original leader
				resp, err = testClient.Operator().RaftLeaderTransfer(leaderID, &api.QueryOptions{})
				if err != nil {
					t.Logf("error initiating leadership transfer to original leader: %s", err)
				} else if !resp.Success {
					t.Log("failed to transfer leadership to original leader")
				}

				t.Log("completed one rotation of leadership swap")
			}
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

				t.Log("Checking KV Data for ", "NodeName", name, "CurrentCount", currentCount)
				checkKV(t, client, int(currentCount))

				if currentCount == numKeysToWrite {
					break
				}
			}
		}(name, client)
	}

	// This goroutine is just to observer leadership changes
	go func() {
		var currentLeader string
		for {
			select {
			case <-ctx.Done():
				return
			default:
				// Make sure the old follower is the leader
				reply, err := apiClients[testNode].Operator().RaftGetConfiguration(&api.QueryOptions{})
				if err != nil {
					t.Logf("error getting raft configuration: %s", err)
					continue
				}
				for _, s := range reply.Servers {
					if s.Leader {
						if currentLeader != s.Node {
							currentLeader = s.Node
							t.Logf("leadership changed to %s", currentLeader)
						}
					}
				}
			}
		}
	}()

	wg.Wait()
	cancel()
}

func writeKV(t *testing.T, client *api.Client, prefix string, index int, counter *atomic.Uint64) error {

	p := &api.KVPair{
		Key: fmt.Sprintf("%s-%010d", prefix, index),
	}
	token := make([]byte, 131072) // 128K size of value
	rand.Read(token)
	p.Value = token

	// We retry until success here
	// We can sometimes get 500 errors when the leadership changes
	retry.Run(t, func(r *retry.R) {
		_, err := client.KV().Put(p, &api.WriteOptions{})
		if err != nil {
			r.Fatal(fmt.Errorf("error writing kv %s: %w", fmt.Sprintf("%s-%010d", prefix, index), err))
		}
	})
	counter.Add(1)

	return nil
}

func checkKV(t *testing.T, client *api.Client, currentCount int) {
	for i := 0; i < numKeysToSeed; i++ {
		_, _, err := client.KV().Get("seed-"+fmt.Sprintf("%010d", i), &api.QueryOptions{})
		require.NoError(t, err, "could not validate", "key-id", "seed-"+fmt.Sprintf("%010d", i))
	}

	for i := 0; i < currentCount; i++ {
		_, _, err := client.KV().Get("test-"+fmt.Sprintf("%010d", i), &api.QueryOptions{})
		require.NoError(t, err, "could not validate", "key-id", "test-"+fmt.Sprintf("%010d", i))
	}
}
