package main

import (
	"fmt"

	"github.com/hashicorp/consul/api"
)

// This script prints out the KV keys for each consul server to verify they are in sync.
func main() {

	cfg := api.DefaultConfig()
	cfg.Address = "localhost:8501"

	leaderClient, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	cfg = api.DefaultConfig()
	cfg.Address = "localhost:8502"

	goodFollowerClient, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	cfg = api.DefaultConfig()
	cfg.Address = "localhost:8503"

	badFollowerClient, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}

	fmt.Println("****Listing keys for leader")
	pairs, _, err := leaderClient.KV().List("", &api.QueryOptions{AllowStale: true})
	if err != nil {
		panic(err)
	}
	if pairs == nil {
		panic("no keys found in leader")
	}
	printPairs(pairs)

	fmt.Println("****Listing keys for good follower")
	pairs, _, err = goodFollowerClient.KV().List("", &api.QueryOptions{AllowStale: true})
	if err != nil {
		panic(err)
	}
	if pairs == nil {
		panic("no keys found in old follower")
	}
	printPairs(pairs)

	fmt.Println("****Listing keys for old follower")
	pairs, _, err = badFollowerClient.KV().List("", &api.QueryOptions{AllowStale: true})
	if err != nil {
		panic(err)
	}
	if pairs == nil {
		panic("no keys found in old follower")
	}
	printPairs(pairs)
}

func printPairs(pairs api.KVPairs) {
	for idx, pair := range pairs {
		fmt.Printf("Idx: %d, Key: %s\n", idx, pair.Key)
		//fmt.Printf("Key: %s, Value: %s\n", pair.Key, pair.Value)
	}
}
