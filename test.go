package main

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

func main() {

	cfg := api.DefaultConfig()
	cfg.Address = "localhost:8501"

	leaderClient, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	cfg = api.DefaultConfig()
	cfg.Address = "localhost:8503"

	oldFollowerClient, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	// We do a Set and a GetOrEmpty to watch the whole transaction fail
	ops := api.TxnOps{
		&api.TxnOp{
			KV: &api.KVTxnOp{
				Verb:  api.KVSet,
				Key:   "test/key1",
				Value: []byte("hello"),
			},
		},
		&api.TxnOp{
			KV: &api.KVTxnOp{
				Verb: api.KVGetOrEmpty,
				Key:  "test/key1",
			},
		},
	}

	ok, _, _, err := leaderClient.Txn().Txn(ops, &api.QueryOptions{})
	if err != nil {
		panic(err)
	}
	if !ok {
		panic("txn failed")
	}

	fmt.Println("Checking the leader for the key")
	pair, _, err := leaderClient.KV().Get("test/key1", &api.QueryOptions{AllowStale: true})
	if err != nil {
		panic("could not find key in leader")
	}
	if pair == nil {
		panic("key not found in leader")
	}

	fmt.Println("Checking the old follower for the key")
	pair, _, err = oldFollowerClient.KV().Get("test/key1", &api.QueryOptions{AllowStale: true})
	if err != nil {
		panic("could not find key in old follower")
	}
	if pair == nil {
		panic("key not found in old follower")
	}
}
