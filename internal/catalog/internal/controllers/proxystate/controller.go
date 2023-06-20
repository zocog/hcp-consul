package proxystate

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/agent/cacheshim"
	"github.com/hashicorp/consul/agent/leafcert"
	catalogv2proxycfg "github.com/hashicorp/consul/agent/proxycfg-sources/catalogv2"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/internal/catalog/internal/types"
	"github.com/hashicorp/consul/internal/controller"
	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Controller(source *catalogv2proxycfg.ConfigSource, leafCertManager *leafcert.Manager, datacenter string) controller.Controller {
	// Create event channel
	ec := make(chan controller.Event)

	// this should eventually be ProxyState resource but we're using workload for now.
	return controller.ForType(types.WorkloadType).
		WithCustomWatch(&controller.Source{Source: ec}, MapLeafCertEvents).
		WithReconciler(&proxyStateReconciler{
			cfgSource:       source,
			leafCertChan:    ec,
			leafCertManager: leafCertManager,
			trackedCerts:    make(map[*pbresource.ID]context.CancelFunc),
			datacenter:      datacenter,
		})
}

func MapLeafCertEvents(ctx context.Context, rt controller.Runtime, event controller.Event) ([]controller.Request, error) {
	var workloadRequests []controller.Request
	// Get cert from event.
	cert, ok := event.Obj.(*structs.IssuedCert)
	if !ok {
		return nil, errors.New("invalid type")
	}

	// do it the dumb way for now. This is probably ok because leaf cert events should happen that often.
	tenancy := &pbresource.Tenancy{
		Namespace: cert.NamespaceOrDefault(),
		Partition: cert.PartitionOrDefault(),
		PeerName:  "local",
	}
	lr, err := rt.Client.List(ctx, &pbresource.ListRequest{Type: types.WorkloadType, Tenancy: tenancy})
	if err != nil {
		return nil, err
	}

	for _, resource := range lr.Resources {
		var workload pbcatalog.Workload
		err = resource.Data.UnmarshalTo(&workload)
		if err != nil {
			return nil, err
		}

		// todo: this should have an identity instead.
		if workload.Identity == cert.Service {
			workloadRequests = append(workloadRequests, controller.Request{
				ID: resource.Id,
			})
		}
	}

	return workloadRequests, nil
}

type proxyStateReconciler struct {
	cfgSource       *catalogv2proxycfg.ConfigSource
	leafCertChan    chan controller.Event
	leafCertManager *leafcert.Manager
	trackedCerts    map[*pbresource.ID]context.CancelFunc
	datacenter      string
}

func (r *proxyStateReconciler) Reconcile(ctx context.Context, rt controller.Runtime, req controller.Request) error {
	// get the workload. We assume the workload is on the mesh.
	res, err := rt.Client.Read(ctx, &pbresource.ReadRequest{Id: req.ID})
	switch {
	case status.Code(err) == codes.NotFound:
		rt.Logger.Trace("workload has been deleted")
		cancel, ok := r.trackedCerts[req.ID]
		if !ok {
			// nothing to do as this workload is not being tracked.
			return nil
		}
		// cancel leaf cert watches
		cancel()
		// stop tracking this workload
		// todo: this should lock the map
		delete(r.trackedCerts, req.ID)
		return nil
	case err != nil:
		rt.Logger.Error("the resource service has returned an unexpected error", "error", err)
		return err
	}

	var workload pbcatalog.Workload
	err = res.Resource.Data.UnmarshalTo(&workload)
	if err != nil {
		return err
	}

	// Notify us of any leaf changes if we haven't seen this workload yet
	if _, ok := r.trackedCerts[req.ID]; !ok {
		certContext, cancel := context.WithCancel(ctx)
		err = r.leafCertManager.NotifyCallback(certContext, &leafcert.ConnectCALeafRequest{
			Datacenter: r.datacenter,
			// todo: this token should come from the cfg src as this should be the token that the proxy uses to connect to us.
			// 		If there's no token, we should skip this as we should get another reconcile event once the proxy connects.
			//Token:          s.token,
			Service:        workload.Identity,
			EnterpriseMeta: acl.EnterpriseMeta{},
		}, "", func(ctx context.Context, event cacheshim.UpdateEvent) {
			cert, ok := event.Result.(*structs.IssuedCert)
			if !ok {
				panic("wrong type")
			}
			controllerEvent := controller.Event{
				Obj: cert,
			}
			r.leafCertChan <- controllerEvent
		})
		r.trackedCerts[req.ID] = cancel
	}

	// Look up the leaf cert in the cache.
	cert, _, err := r.leafCertManager.Get(ctx, &leafcert.ConnectCALeafRequest{
		Datacenter: r.datacenter,
		//Token:          s.token,
		Service:        workload.Identity,
		EnterpriseMeta: acl.EnterpriseMeta{},
	})
	if err != nil {
		return err
	}

	// todo: print the cert for this PoC
	fmt.Println("==============received cert", cert)

	return nil
}
