// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package controllers

import (
	"github.com/hashicorp/consul/internal/catalog"
	"github.com/hashicorp/consul/internal/controller"
	"github.com/hashicorp/consul/internal/mesh/internal/cache/sidecarproxycache"
	"github.com/hashicorp/consul/internal/mesh/internal/controllers/sidecarproxy"
	"github.com/hashicorp/consul/internal/mesh/internal/controllers/xds"
	"github.com/hashicorp/consul/internal/mesh/internal/mappers/sidecarproxymapper"
	"github.com/hashicorp/consul/internal/mesh/internal/types"
	"github.com/hashicorp/consul/internal/resource/mappers/bimapper"
)

type Dependencies struct {
	TrustBundleFetcher xds.TrustBundleFetcher
	TrustDomainFetcher sidecarproxy.TrustDomainFetcher
	LocalDatacenter    string
}

func Register(mgr *controller.Manager, deps Dependencies) {
	{
		mapper := bimapper.New(types.ProxyStateTemplateType, catalog.ServiceEndpointsType)
		// TODO: Pass in a "real" updater once proxy tracker work has completed.
		mgr.Register(xds.Controller(mapper, nil, deps.TrustBundleFetcher))
	}
	{
		destinationsCache := sidecarproxycache.NewDestinationsCache()
		proxyCfgCache := sidecarproxycache.NewProxyConfigurationCache()
		m := sidecarproxymapper.New(destinationsCache, proxyCfgCache)

		mgr.Register(
			sidecarproxy.Controller(destinationsCache, proxyCfgCache, m, deps.TrustDomainFetcher, deps.LocalDatacenter),
		)
	}
}
