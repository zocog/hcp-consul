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
	ProxyConfigUpdater       catalogv2proxycfg.Updater
	Datacenter               string
}

func Register(mgr *controller.Manager, deps Dependencies) {
	mgr.Register(nodehealth.NodeHealthController())
	mgr.Register(workloadhealth.WorkloadHealthController(deps.WorkloadHealthNodeMapper))
	mgr.Register(endpoints.ServiceEndpointsController(deps.EndpointsWorkloadMapper))
	if deps.ProxyConfigUpdater == nil {
		panic("register: proxy config updater is nil")
	}
	if deps.LeafCertManager == nil {
		panic("register: leaf cert manager is nil")
	}
	mgr.Register(proxystate.Controller(deps.ProxyConfigUpdater, deps.LeafCertManager, deps.Datacenter))
}
