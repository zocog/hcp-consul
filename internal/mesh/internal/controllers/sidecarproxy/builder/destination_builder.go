// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package builder

import (
	"fmt"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/hashicorp/consul/agent/connect"
	"github.com/hashicorp/consul/envoyextensions/xdscommon"
	"github.com/hashicorp/consul/internal/mesh/internal/types/intermediate"
	"github.com/hashicorp/consul/internal/resource"
	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v1alpha1"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1/pbproxystate"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

func (b *Builder) BuildDestinations(destinations []*intermediate.Destination) *Builder {
	if b.proxyCfg.GetDynamicConfig() != nil &&
		b.proxyCfg.DynamicConfig.Mode == pbmesh.ProxyMode_PROXY_MODE_TRANSPARENT {

		b.addOutboundListener(b.proxyCfg.DynamicConfig.TransparentProxy.OutboundListenerPort)
	}

	for _, destination := range destinations {
		if destination.Explicit != nil {
			b.buildExplicitDestination(destination)
		} else {
			b.buildImplicitDestination(destination)
		}
	}

	return b
}

func (b *Builder) getLastBuiltListener() *pbproxystate.Listener {
	lastBuiltIndex := len(b.proxyStateTemplate.ProxyState.Listeners) - 1
	return b.proxyStateTemplate.ProxyState.Listeners[lastBuiltIndex]
}

func (b *Builder) buildExplicitDestination(destination *intermediate.Destination) *Builder {
	b.addOutboundDestinationListener(destination.Explicit)
	for _, endpoint := range destination.ServiceEndpoints.Endpoints.Endpoints {
		for portName, port := range endpoint.Ports {
			sni := DestinationSNI(destination.Explicit.DestinationRef, destination.Explicit.Datacenter, b.trustDomain)
			clusterName := fmt.Sprintf("%s.%s", portName, sni)
			statPrefix := DestinationStatPrefix(destination.Explicit.DestinationRef, portName, destination.Explicit.Datacenter)
			if isMeshPort(port) {
				b.addEndpointsRef(sni, destination.ServiceEndpoints.Resource.Id, portName)
			} else if port.GetProtocol() != pbcatalog.Protocol_PROTOCOL_TCP {
				//only implementing L4 at the moment
				continue
			} else {
				b.addRouterWithIPAndPortMatch(clusterName, statPrefix, nil, destination.VirtualIPs).
					addCluster(clusterName, sni, portName, destination.Identities)
			}
		}
	}

	return b
}

func (b *Builder) buildImplicitDestination(destination *intermediate.Destination) *Builder {
	for _, endpoint := range destination.ServiceEndpoints.Endpoints.Endpoints {
		for portName, port := range endpoint.Ports {
			serviceRef := resource.Reference(destination.ServiceEndpoints.Resource.Owner, "")
			sni := DestinationSNI(serviceRef, b.localDatacenter, b.trustDomain)
			statPrefix := DestinationStatPrefix(serviceRef, portName, b.localDatacenter)
			if isMeshPort(port) {
				b.addEndpointsRef(sni, destination.ServiceEndpoints.Resource.Id, portName)
			} else if port.GetProtocol() != pbcatalog.Protocol_PROTOCOL_TCP {
				//only implementing L4 at the moment
				continue
			} else {
				clusterName := fmt.Sprintf("%s.%s", portName, sni)
				b.addRouterWithIPAndPortMatch(clusterName, statPrefix, port, destination.VirtualIPs).
					addCluster(clusterName, sni, portName, destination.Identities)
			}
		}
	}

	return b
}

func (b *Builder) addOutboundDestinationListener(explicit *pbmesh.Upstream) *Builder {
	listener := &pbproxystate.Listener{
		Direction: pbproxystate.Direction_DIRECTION_OUTBOUND,
	}

	// Create outbound listener address.
	switch explicit.ListenAddr.(type) {
	case *pbmesh.Upstream_IpPort:
		destinationAddr := explicit.ListenAddr.(*pbmesh.Upstream_IpPort)
		listener.BindAddress = &pbproxystate.Listener_HostPort{
			HostPort: &pbproxystate.HostPortAddress{
				Host: destinationAddr.IpPort.Ip,
				Port: destinationAddr.IpPort.Port,
			},
		}
		listener.Name = DestinationListenerName(explicit.DestinationRef.Name, explicit.DestinationPort, destinationAddr.IpPort.Ip, destinationAddr.IpPort.Port)
	case *pbmesh.Upstream_Unix:
		destinationAddr := explicit.ListenAddr.(*pbmesh.Upstream_Unix)
		listener.BindAddress = &pbproxystate.Listener_UnixSocket{
			UnixSocket: &pbproxystate.UnixSocketAddress{
				Path: destinationAddr.Unix.Path,
				Mode: destinationAddr.Unix.Mode,
			},
		}
		listener.Name = DestinationListenerName(explicit.DestinationRef.Name, explicit.DestinationPort, destinationAddr.Unix.Path, 0)
	}

	return b.addListener(listener)
}

func (b *Builder) addOutboundListener(port uint32) *Builder {
	listener := &pbproxystate.Listener{
		Name:      xdscommon.OutboundListenerName,
		Direction: pbproxystate.Direction_DIRECTION_OUTBOUND,
		BindAddress: &pbproxystate.Listener_HostPort{
			HostPort: &pbproxystate.HostPortAddress{
				Host: "127.0.0.1",
				Port: port,
			},
		},
		Capabilities: []pbproxystate.Capability{pbproxystate.Capability_CAPABILITY_TRANSPARENT},
	}

	return b.addListener(listener)
}

func (b *Builder) addRouterDestination(router *pbproxystate.Router, clusterName, statPrefix string, port *pbcatalog.WorkloadPort) {
	switch port.GetProtocol() {
	case pbcatalog.Protocol_PROTOCOL_TCP:
		router.Destination = &pbproxystate.Router_L4{
			L4: &pbproxystate.L4Destination{
				Name:       clusterName,
				StatPrefix: statPrefix,
			},
		}
	case pbcatalog.Protocol_PROTOCOL_HTTP:
		router.Destination = &pbproxystate.Router_L7{
			L7: &pbproxystate.L7Destination{
				Name:       clusterName,
				StatPrefix: statPrefix,
			},
		}
	}
}
func (b *Builder) addRouterWithIPAndPortMatch(clusterName, statPrefix string, port *pbcatalog.WorkloadPort, vips []string) *Builder {
	listener := b.getLastBuiltListener()

	// For explicit destinations, we have no filter chain match, and filters are based on port protocol.
	router := &pbproxystate.Router{}
	b.addRouterDestination(router, clusterName, statPrefix, port)

	if router.Destination != nil {
		if (port != nil || len(vips) > 0) && router.Match == nil {
			router.Match = &pbproxystate.Match{}
		}
		if port != nil {
			router.Match.DestinationPort = &wrapperspb.UInt32Value{Value: port.GetPort()}
		}
		for _, vip := range vips {
			router.Match.PrefixRanges = append(router.Match.PrefixRanges, &pbproxystate.CidrRange{
				AddressPrefix: vip,
				PrefixLen:     &wrapperspb.UInt32Value{Value: 32},
			})
		}
		listener.Routers = append(listener.Routers, router)
	}

	return b
}

func (b *Builder) addCluster(clusterName, sni, portName string, destinationIdentities []*pbresource.Reference) {
	var spiffeIDs []string
	for _, identity := range destinationIdentities {
		spiffeIDs = append(spiffeIDs, connect.SpiffeIDFromIdentityRef(b.trustDomain, identity))
	}

	// Create destination cluster.
	cluster := &pbproxystate.Cluster{
		Group: &pbproxystate.Cluster_EndpointGroup{
			EndpointGroup: &pbproxystate.EndpointGroup{
				Group: &pbproxystate.EndpointGroup_Dynamic{
					Dynamic: &pbproxystate.DynamicEndpointGroup{
						Config: &pbproxystate.DynamicEndpointGroupConfig{
							DisablePanicThreshold: true,
						},
						OutboundTls: &pbproxystate.TransportSocket{
							ConnectionTls: &pbproxystate.TransportSocket_OutboundMesh{
								OutboundMesh: &pbproxystate.OutboundMeshMTLS{
									IdentityKey: b.proxyStateTemplate.ProxyState.Identity.Name,
									ValidationContext: &pbproxystate.MeshOutboundValidationContext{
										SpiffeIds: spiffeIDs,
									},
									Sni: sni,
								},
							},
							AlpnProtocols: []string{
								fmt.Sprintf("consul~%s", portName),
							},
						},
					},
				},
			},
		},
	}

	b.proxyStateTemplate.ProxyState.Clusters[clusterName] = cluster
}

func (b *Builder) addEndpointsRef(clusterName string, serviceEndpointsID *pbresource.ID, destinationPort string) {
	b.proxyStateTemplate.RequiredEndpoints[clusterName] = &pbproxystate.EndpointRef{
		Id:   serviceEndpointsID,
		Port: destinationPort,
	}
}
