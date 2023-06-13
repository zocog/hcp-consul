// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package controllers

import (
	"github.com/hashicorp/consul/agent/cacheshim"
	"github.com/hashicorp/consul/agent/leafcert"
	catalogv2proxycfg "github.com/hashicorp/consul/agent/proxycfg-sources/catalogv2"
	"github.com/hashicorp/consul/internal/catalog/internal/controllers/endpoints"
	"github.com/hashicorp/consul/internal/catalog/internal/controllers/nodehealth"
	"github.com/hashicorp/consul/internal/catalog/internal/controllers/proxystate"
	"github.com/hashicorp/consul/internal/catalog/internal/controllers/workloadhealth"
	"github.com/hashicorp/consul/internal/controller"
)

type Dependencies struct {
	WorkloadHealthNodeMapper workloadhealth.NodeMapper
	EndpointsWorkloadMapper  endpoints.WorkloadMapper
	Cache                    cacheshim.Cache
	LeafCertManager          *leafcert.Manager
	CfgSource                catalogv2proxycfg.Watcher
}

func Register(mgr *controller.Manager, deps Dependencies) {
	mgr.Register(nodehealth.NodeHealthController())
	mgr.Register(workloadhealth.WorkloadHealthController(deps.WorkloadHealthNodeMapper))
	mgr.Register(endpoints.ServiceEndpointsController(deps.EndpointsWorkloadMapper))
	cfgSrc, ok := deps.CfgSource.(*catalogv2proxycfg.ConfigSource)
	if !ok {
		panic("this should never happen")
	}
	mgr.Register(proxystate.ProxyStateController(deps.Cache, cfgSrc, deps.LeafCertManager))
}

// type LeafCertManager interface {
// Get(ctx context.Context, req *ConnectCALeafRequest) (*structs.IssuedCert, cache.ResultMeta, error)
// Notify(ctx context.Context, req *ConnectCALeafRequest, correlationID string, ...) error
// NotifyCallback(ctx context.Context, req *ConnectCALeafRequest, correlationID string, ...) error
// }
