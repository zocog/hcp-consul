// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hcp

import (
	hcpclient "github.com/hashicorp/consul/agent/hcp/client"
	"github.com/hashicorp/consul/agent/hcp/config"
	"github.com/hashicorp/consul/agent/hcp/scada"
	"github.com/hashicorp/go-hclog"
)

// Deps contains the interfaces that the rest of Consul core depends on for HCP integration.
type Deps struct {
	Client        hcpclient.Client
	Provider      scada.Provider
	MetricsClient hcpclient.MetricsClient
}

func NewDeps(cfg config.CloudConfig, logger hclog.Logger) (d Deps, err error) {
	d.Client, err = hcpclient.NewClient(cfg)
	if err != nil {
		return
	}

	d.Provider, err = scada.New(cfg, logger.Named("hcp.scada"))
	if err != nil {
		return
	}

	metricsClientOpts := &hcpclient.TelemetryClientCfg{
		Logger:           logger,
		CloudCfg:         &cfg,
		EndpointProvider: hcpclient.NewRefreshableTelemetryEndpointProvider(d.Client),
	}
	d.MetricsClient, err = hcpclient.NewMetricsClient(metricsClientOpts)
	if err != nil {
		return
	}

	return
}
