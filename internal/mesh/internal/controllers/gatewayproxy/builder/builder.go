// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package builder

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul/agent/connect"
	"github.com/hashicorp/consul/envoyextensions/xdscommon"
	"github.com/hashicorp/consul/internal/mesh/internal/controllers/gatewayproxy/fetcher"
	"github.com/hashicorp/consul/internal/mesh/internal/types"
	"github.com/hashicorp/consul/internal/resource"
	pbauth "github.com/hashicorp/consul/proto-public/pbauth/v2beta1"
	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v2beta1"
	meshv2beta1 "github.com/hashicorp/consul/proto-public/pbmesh/v2beta1"
	"github.com/hashicorp/consul/proto-public/pbmesh/v2beta1/pbproxystate"
	pbmulticluster "github.com/hashicorp/consul/proto-public/pbmulticluster/v2beta1"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type proxyStateTemplateBuilder struct {
	workload         *types.DecodedWorkload
	dataFetcher      *fetcher.Fetcher
	dc               string
	exportedServices *types.DecodedComputedExportedServices
	logger           hclog.Logger
	trustDomain      string
	remoteGatewayIDs []*pbresource.ID
}

func NewProxyStateTemplateBuilder(workload *types.DecodedWorkload, exportedServices *types.DecodedComputedExportedServices, logger hclog.Logger, dataFetcher *fetcher.Fetcher, dc, trustDomain string, remoteMeshGateways []*pbresource.ID) *proxyStateTemplateBuilder {
	return &proxyStateTemplateBuilder{
		workload:         workload,
		dataFetcher:      dataFetcher,
		dc:               dc,
		exportedServices: exportedServices,
		logger:           logger,
		remoteGatewayIDs: remoteMeshGateways,
		trustDomain:      trustDomain,
	}
}

func (b *proxyStateTemplateBuilder) identity() *pbresource.Reference {
	return &pbresource.Reference{
		Name:    b.workload.Data.Identity,
		Tenancy: b.workload.Id.Tenancy,
		Type:    pbauth.WorkloadIdentityType,
	}
}

func (b *proxyStateTemplateBuilder) listeners() []*pbproxystate.Listener {
	var listeners []*pbproxystate.Listener
	var address *pbcatalog.WorkloadAddress

	// TODO: NET-7260 we think there should only ever be a single address for a gateway,
	// need to validate this
	if len(b.workload.Data.Addresses) > 0 {
		address = b.workload.Data.Addresses[0]
	}

	// if the address defines no ports we assume the intention is to bind to all
	// ports on the workload
	if len(address.Ports) == 0 {
		for _, workloadPort := range b.workload.Data.Ports {
			listeners = append(listeners, b.buildListener(address, workloadPort.Port))
		}
		return listeners
	}

	for _, portName := range address.Ports {
		workloadPort, ok := b.workload.Data.Ports[portName]
		if !ok {
			b.logger.Trace("port does not exist for workload", "port name", portName)
			continue
		}

		listeners = append(listeners, b.buildListener(address, workloadPort.Port))
	}

	return listeners
}

func (b *proxyStateTemplateBuilder) buildListener(address *pbcatalog.WorkloadAddress, port uint32) *pbproxystate.Listener {
	return &pbproxystate.Listener{
		Name:      xdscommon.PublicListenerName,
		Direction: pbproxystate.Direction_DIRECTION_INBOUND,
		BindAddress: &pbproxystate.Listener_HostPort{
			HostPort: &pbproxystate.HostPortAddress{
				Host: address.Host,
				Port: port,
			},
		},
		Capabilities: []pbproxystate.Capability{
			pbproxystate.Capability_CAPABILITY_L4_TLS_INSPECTION,
		},
		DefaultRouter: &pbproxystate.Router{
			Destination: &pbproxystate.Router_L4{
				L4: &pbproxystate.L4Destination{
					Destination: &pbproxystate.L4Destination_Cluster{
						Cluster: &pbproxystate.DestinationCluster{
							Name: "",
						},
					},
					StatPrefix: "prefix",
				},
			},
		},
		Routers: b.routers(),
	}
}

// routers loops through the ports and consumers for each exported service and generates
// a pbproxystate.Router matching the SNI to the target cluster. The target port name
// will be included in the ALPN. The targeted cluster will marry this port name with the SNI.
func (b *proxyStateTemplateBuilder) routers() []*pbproxystate.Router {
	var routers []*pbproxystate.Router

	if b.exportedServices == nil {
		return routers
	}

	// Routers handling incoming traffic from another partition
	for _, exportedService := range b.exportedServices.Data.Services {
		serviceID := resource.IDFromReference(exportedService.TargetRef)
		service, err := b.dataFetcher.FetchService(context.Background(), serviceID)
		if err != nil {
			b.logger.Trace("error reading exported service", "error", err)
			continue
		} else if service == nil {
			b.logger.Trace("service does not exist, skipping router", "service", serviceID)
			continue
		}

		for _, port := range service.Data.Ports {
			for _, consumer := range exportedService.Consumers {
				routers = append(routers, &pbproxystate.Router{
					Match: &pbproxystate.Match{
						AlpnProtocols: []string{alpnProtocol(port.TargetPort)},
						ServerNames:   []string{b.sniForExportedService(exportedService.TargetRef, consumer)},
					},
					Destination: &pbproxystate.Router_L4{
						L4: &pbproxystate.L4Destination{
							Destination: &pbproxystate.L4Destination_Cluster{
								Cluster: &pbproxystate.DestinationCluster{
									Name: b.clusterNameForExportedService(exportedService.TargetRef, consumer, port.TargetPort),
								},
							},
							StatPrefix: "prefix",
						},
					},
				})
			}
		}
	}

	// Routers handling incoming traffic from the local partition
	for _, remoteGatewayID := range b.remoteGatewayIDs {
		serviceID := resource.ReplaceType(pbcatalog.ServiceType, remoteGatewayID)
		service, err := b.dataFetcher.FetchService(context.Background(), serviceID)
		if err != nil {
			b.logger.Trace("error reading exported service", "error", err)
			continue
		} else if service == nil {
			b.logger.Trace("service does not exist, skipping router", "service", serviceID)
			continue
		}

		routers = append(routers, &pbproxystate.Router{
			Match: &pbproxystate.Match{
				ServerNames: []string{
					fmt.Sprintf("*.%s", b.clusterNameForRemoteGateway(remoteGatewayID)),
				},
			},
			Destination: &pbproxystate.Router_L4{
				L4: &pbproxystate.L4Destination{
					Destination: &pbproxystate.L4Destination_Cluster{
						Cluster: &pbproxystate.DestinationCluster{
							Name: b.clusterNameForRemoteGateway(remoteGatewayID),
						},
					},
					StatPrefix: "prefix",
				},
			},
		})
	}

	return routers
}

func (b *proxyStateTemplateBuilder) clusters() map[string]*pbproxystate.Cluster {
	clusters := map[string]*pbproxystate.Cluster{}

	if b.exportedServices == nil {
		return clusters
	}

	// Clusters handling incoming traffic from another partition
	for _, exportedService := range b.exportedServices.Data.Services {
		serviceID := resource.IDFromReference(exportedService.TargetRef)
		service, err := b.dataFetcher.FetchService(context.Background(), serviceID)
		if err != nil {
			b.logger.Trace("error reading exported service", "error", err)
			continue
		} else if service == nil {
			b.logger.Trace("service does not exist, skipping router", "service", serviceID)
			continue
		}

		for _, port := range service.Data.Ports {
			for _, consumer := range exportedService.Consumers {
				clusterName := b.clusterNameForExportedService(exportedService.TargetRef, consumer, port.TargetPort)
				clusters[clusterName] = &pbproxystate.Cluster{
					Name:     clusterName,
					Protocol: pbproxystate.Protocol_PROTOCOL_TCP, // TODO
					Group: &pbproxystate.Cluster_EndpointGroup{
						EndpointGroup: &pbproxystate.EndpointGroup{
							Group: &pbproxystate.EndpointGroup_Dynamic{},
						},
					},
					AltStatName: "prefix",
				}
			}
		}
	}

	// Clusters handling incoming traffic from the local partition
	for _, remoteGatewayID := range b.remoteGatewayIDs {
		serviceID := resource.ReplaceType(pbcatalog.ServiceType, remoteGatewayID)
		service, err := b.dataFetcher.FetchService(context.Background(), serviceID)
		if err != nil {
			b.logger.Trace("error reading exported service", "error", err)
			continue
		} else if service == nil {
			b.logger.Trace("service does not exist, skipping router", "service", serviceID)
			continue
		}

		clusterName := b.clusterNameForRemoteGateway(remoteGatewayID)
		clusters[clusterName] = &pbproxystate.Cluster{
			Name:     clusterName,
			Protocol: pbproxystate.Protocol_PROTOCOL_TCP, // TODO
			Group: &pbproxystate.Cluster_EndpointGroup{
				EndpointGroup: &pbproxystate.EndpointGroup{
					Group: &pbproxystate.EndpointGroup_Dynamic{},
				},
			},
			AltStatName: "prefix",
		}
	}

	return clusters
}

func (b *proxyStateTemplateBuilder) endpoints() map[string]*pbproxystate.Endpoints {
	// TODO NET-6431
	return nil
}

func (b *proxyStateTemplateBuilder) routes() map[string]*pbproxystate.Route {
	// TODO NET-6428
	return nil
}

func (b *proxyStateTemplateBuilder) Build() *meshv2beta1.ProxyStateTemplate {
	return &meshv2beta1.ProxyStateTemplate{
		ProxyState: &meshv2beta1.ProxyState{
			Identity:  b.identity(),
			Listeners: b.listeners(),
			Endpoints: b.endpoints(),
			Clusters:  b.clusters(),
			Routes:    b.routes(),
		},
		RequiredEndpoints:        b.requiredEndpoints(),
		RequiredLeafCertificates: make(map[string]*pbproxystate.LeafCertificateRef),
		RequiredTrustBundles:     make(map[string]*pbproxystate.TrustBundleRef),
	}
}

// requiredEndpoints loops through the consumers for each exported service
// and adds a pbproxystate.EndpointRef to be hydrated for each cluster.
func (b *proxyStateTemplateBuilder) requiredEndpoints() map[string]*pbproxystate.EndpointRef {
	requiredEndpoints := make(map[string]*pbproxystate.EndpointRef)
	if b.exportedServices == nil {
		return requiredEndpoints
	}

	// Endpoints for clusters handling incoming traffic from another partition
	for _, exportedService := range b.exportedServices.Data.Services {
		serviceID := resource.IDFromReference(exportedService.TargetRef)
		service, err := b.dataFetcher.FetchService(context.Background(), serviceID)
		if err != nil {
			b.logger.Trace("error reading exported service", "error", err)
			continue
		} else if service == nil {
			b.logger.Trace("service does not exist, skipping router", "service", serviceID)
			continue
		}

		for _, port := range service.Data.Ports {
			for _, consumer := range exportedService.Consumers {
				clusterName := b.clusterNameForExportedService(exportedService.TargetRef, consumer, port.TargetPort)
				requiredEndpoints[clusterName] = &pbproxystate.EndpointRef{
					Id:   resource.ReplaceType(pbcatalog.ServiceEndpointsType, serviceID),
					Port: port.TargetPort,
				}
			}
		}
	}

	// Endpoints for clusters handling incoming traffic from the local partition
	for _, remoteGatewayID := range b.remoteGatewayIDs {
		serviceID := resource.ReplaceType(pbcatalog.ServiceType, remoteGatewayID)
		service, err := b.dataFetcher.FetchService(context.Background(), serviceID)
		if err != nil {
			b.logger.Trace("error reading exported service", "error", err)
			continue
		} else if service == nil {
			b.logger.Trace("service does not exist, skipping router", "service", serviceID)
			continue
		}

		for _, port := range service.Data.Ports {
			clusterName := b.clusterNameForRemoteGateway(remoteGatewayID)
			requiredEndpoints[clusterName] = &pbproxystate.EndpointRef{
				Id:   resource.ReplaceType(pbcatalog.ServiceEndpointsType, serviceID),
				Port: port.TargetPort,
			}
		}
	}

	return requiredEndpoints
}

// clusterNameForExportedService generates a cluster name for a given service
// that is being exported from the local partition to a remote partition. This
// partition may reside in the same datacenter or in a remote datacenter.
func (b *proxyStateTemplateBuilder) clusterNameForExportedService(serviceRef *pbresource.Reference, consumer *pbmulticluster.ComputedExportedServiceConsumer, port string) string {
	return fmt.Sprintf("%s.%s", port, b.sniForExportedService(serviceRef, consumer))
}

func (b *proxyStateTemplateBuilder) sniForExportedService(serviceRef *pbresource.Reference, consumer *pbmulticluster.ComputedExportedServiceConsumer) string {
	switch tConsumer := consumer.Tenancy.(type) {
	case *pbmulticluster.ComputedExportedServiceConsumer_Partition:
		return connect.ServiceSNI(serviceRef.Name, "", serviceRef.Tenancy.Namespace, tConsumer.Partition, b.dc, b.trustDomain)
	case *pbmulticluster.ComputedExportedServiceConsumer_Peer:
		return connect.PeeredServiceSNI(serviceRef.Name, serviceRef.Tenancy.Namespace, serviceRef.Tenancy.Partition, tConsumer.Peer, b.trustDomain)
	default:
		return ""
	}
}

// clusterNameForRemoteGateway generates a cluster name for a given remote mesh
// gateway. This will be used to route traffic from the local partition to the mesh
// gateway for a remote partition.
func (b *proxyStateTemplateBuilder) clusterNameForRemoteGateway(remoteGatewayID *pbresource.ID) string {
	return connect.GatewaySNI(remoteGatewayID.Tenancy.PeerName, remoteGatewayID.Tenancy.Partition, b.trustDomain)
}

func alpnProtocol(portName string) string {
	return fmt.Sprintf("consul~%s", portName)
}
