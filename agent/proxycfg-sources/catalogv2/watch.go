package catalogv2

import (
	"fmt"

	"github.com/hashicorp/consul/agent/grpc-external/limiter"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/internal/proxystate"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type Watcher interface {
	Watch(proxyID structs.ServiceID, nodeName string, token string) (<-chan *proxystate.FullProxyState, limiter.SessionTerminatedChan, func(), error)
}

type ConfigSource struct {
	proxies map[*pbresource.ID]proxyWatchData
}

type proxyWatchData struct {
	notify   chan *proxystate.FullProxyState
	state    *proxystate.FullProxyState
	token    string
	nodeName string
}

func NewConfigSource() *ConfigSource {
	return &ConfigSource{
		proxies: make(map[*pbresource.ID]proxyWatchData),
	}
}

func (r *ConfigSource) Watch(id structs.ServiceID, nodeName string, token string) (<-chan *proxystate.FullProxyState, limiter.SessionTerminatedChan, func(), error) {
	// get this ID convert to pbresource ID
	workloadType := &pbresource.Type{
		Group:        "catalog",
		GroupVersion: "v1alpha1",
		Kind:         "workload",
	}
	rid := &pbresource.ID{
		Type: workloadType,
		Tenancy: &pbresource.Tenancy{
			Namespace: id.NamespaceOrDefault(),
			Partition: id.PartitionOrDefault(),
		},
		Name: id.ID,
	}

	proxyStateChan := make(chan *proxystate.FullProxyState)
	// todo: lock it
	r.proxies[rid] = proxyStateChan

	// send it if it's ready here as well.

	// todo: create a cancel function and have a way to stop watching this if the
	// 	we might need to start an async watch here to watch for updates on this channel.

	// todo: send a proxy state if there's one already in the cache.

	return proxyStateChan, nil, nil, nil
}

func (r *ConfigSource) PushChange(id *pbresource.ID, snapshot *proxystate.FullProxyState) error {
	// this is not thread safe!
	// todo: this needs to implement rate limiting like our current code
	if ch, ok := r.proxies[id]; ok {
		ch <- snapshot
	} else {
		return fmt.Errorf("could not send changes to proxy")
	}

	// todo: always add states to the cache.
	return nil
}

func (r *ConfigSource) Shutdown() {
	// todo: implement
}
