package proxystate

import (
	"context"

	catalogv2proxycfg "github.com/hashicorp/consul/agent/proxycfg-sources/catalogv2"
	"github.com/hashicorp/consul/internal/catalog/internal/types"
	"github.com/hashicorp/consul/internal/controller"
	"github.com/hashicorp/consul/internal/proxystate"
)

func ProxyStateController(source *catalogv2proxycfg.ConfigSource) controller.Controller {
	// this should eventually be ProxyState resource but we're using workload for now.
	return controller.ForType(types.WorkloadType).
		WithReconciler(&proxyStateReconciler{cfgSource: source})
}

type proxyStateReconciler struct {
	cfgSource *catalogv2proxycfg.ConfigSource
}

func (r *proxyStateReconciler) Reconcile(ctx context.Context, rt controller.Runtime, req controller.Request) error {
	var cfgSnap *proxystate.FullProxyState
	err := r.cfgSource.PushChange(req.ID, cfgSnap)
	if err != nil {
		return err
	}

	return nil
}
