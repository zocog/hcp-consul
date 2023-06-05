package server

import (
	"context"
	"sync"

	"github.com/hashicorp/consul/proto-public/pbresource"
	pb "github.com/hashicorp/consul/proto-public/pbserver/v1alpha1"
)

type FlagWatcher struct {
	client pbresource.ResourceServiceClient

	mu      sync.RWMutex
	servers map[string]*pb.ServerMetadata
	ch      chan struct{}
}

func NewFlagWatcher(client pbresource.ResourceServiceClient) *FlagWatcher {
	return &FlagWatcher{
		client:  client,
		servers: make(map[string]*pb.ServerMetadata),
		ch:      make(chan struct{}, 1),
	}
}

func (w *FlagWatcher) Run(ctx context.Context) {
	watch, err := w.client.WatchList(ctx, &pbresource.WatchListRequest{
		Type: MetadataType,
		Tenancy: &pbresource.Tenancy{
			Partition: "default",
			PeerName:  "local",
			Namespace: "default",
		},
	})
	if err != nil {
		// TODO: handle this error.
		return
	}

	for {
		event, err := watch.Recv()
		if err != nil {
			// TODO: handle this error.
			return
		}

		switch event.Operation {
		case pbresource.WatchEvent_OPERATION_UPSERT:
			var meta pb.ServerMetadata
			if err := event.Resource.Data.UnmarshalTo(&meta); err != nil {
				// TODO: handle this error
				continue
			}

			w.mu.Lock()
			w.servers[event.Resource.Id.Name] = &meta
			w.mu.Unlock()
		case pbresource.WatchEvent_OPERATION_DELETE:
			w.mu.Lock()
			delete(w.servers, event.Resource.Id.Name)
			w.mu.Unlock()
		default:
			continue
		}

		select {
		case w.ch <- struct{}{}:
		default:
		}
	}
}

func (w *FlagWatcher) Supported(flag string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// TODO: filter out stale servers/compare to autopilot health.
	if len(w.servers) == 0 {
		return false
	}

	for _, server := range w.servers {
		var seen bool
		for _, feature := range server.Features {
			if feature == flag {
				seen = true
				break
			}
		}
		if !seen {
			return false
		}
	}

	return true
}

func (w *FlagWatcher) Change() <-chan struct{} { return w.ch }
