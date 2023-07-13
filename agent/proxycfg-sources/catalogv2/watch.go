package catalogv2

import (
	"fmt"

	"github.com/hashicorp/consul/agent/grpc-external/limiter"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/internal/controller"
	"github.com/hashicorp/consul/internal/proxystate"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type Watcher interface {
	Watch(proxyID structs.ServiceID, nodeName string, token string) (<-chan *proxystate.FullProxyState, limiter.SessionTerminatedChan, func(), error)
}
type Updater interface {
	PushChange(id *pbresource.ID, snapshot *proxystate.FullProxyState) error
	EventChannel() chan controller.Event
}

// ProxyTracker implements the Watcher and Updater interfaces. The Watcher is used by the xds server to add a new proxy
// to this server, and get back a channel for updates. The Updater is used by the ProxyState controller running on the
// server to push FullProxyState updates to the notify channel.
type ProxyTracker struct {
	// proxies is a cache of the proxies connected to this server and configuration information for each one.
	proxies            map[*pbresource.ID]*proxyWatchData
	newProxyConnection chan controller.Event
}

type proxyWatchData struct {
	notify   chan *proxystate.FullProxyState
	state    *proxystate.FullProxyState
	token    string
	nodeName string
}

func NewProxyTracker() *ProxyTracker {
	return &ProxyTracker{
		proxies:            make(map[*pbresource.ID]*proxyWatchData),
		newProxyConnection: make(chan controller.Event),
	}
}

type resourceId struct {
	id *pbresource.ID
}

func (r resourceId) Key() string {
	return r.id.String()
}
func (r *ProxyTracker) EventChannel() chan controller.Event {
	return r.newProxyConnection
}
func (r *ProxyTracker) Watch(id structs.ServiceID, nodeName string, token string) (<-chan *proxystate.FullProxyState, limiter.SessionTerminatedChan, func(), error) {
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
	r.proxies[rid] = &proxyWatchData{
		notify: proxyStateChan,
	}

	// Send an event to the controller
	controllerEvent := controller.Event{
		Obj: resourceId{rid},
	}
	r.newProxyConnection <- controllerEvent
	// send it if it's ready here as well.

	// todo: create a cancel function and have a way to stop watching this if the
	// 	we might need to start an async watch here to watch for updates on this channel.

	// todo: send a proxy state if there's one already in the cache.

	return proxyStateChan, nil, nil, nil
}

func (r *ProxyTracker) PushChange(id *pbresource.ID, snapshot *proxystate.FullProxyState) error {
	// this is not thread safe!
	// todo: this needs to implement rate limiting like our current code
	if d, ok := r.proxies[id]; ok {
		d.notify <- snapshot
	} else {
		return fmt.Errorf("could not send changes to proxy")
	}

	// todo: always add states to the cache.
	return nil
}

func (r *ProxyTracker) Shutdown() {
	// todo: implement
}
