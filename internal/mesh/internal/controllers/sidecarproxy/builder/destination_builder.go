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

type servicePortInfo struct {
	meshPortName string
	meshPort     *pbcatalog.WorkloadPort
	servicePorts map[string]*pbcatalog.WorkloadPort
}

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

func (b *Builder) buildExplicitDestination(destination *intermediate.Destination) *Builder {
	serviceRef := destination.Explicit.DestinationRef
	sni := DestinationSNI(serviceRef, b.localDatacenter, b.trustDomain)
	portInfo := getServicePortInfo(destination.ServiceEndpoints.Endpoints)

	return b.addOutboundDestinationListener(destination.Explicit).
		addEndpointsRef(sni, destination.ServiceEndpoints.Resource.Id, portInfo.meshPortName).
		addRouters(portInfo, destination, serviceRef, sni, b.localDatacenter, false).
		addClusters(portInfo, destination, sni)
}

func (b *Builder) buildImplicitDestination(destination *intermediate.Destination) *Builder {
	serviceRef := resource.Reference(destination.ServiceEndpoints.Resource.Owner, "")
	sni := DestinationSNI(serviceRef, b.localDatacenter, b.trustDomain)
	portInfo := getServicePortInfo(destination.ServiceEndpoints.Endpoints)

	return b.addEndpointsRef(sni, destination.ServiceEndpoints.Resource.Id, portInfo.meshPortName).
		addRouters(portInfo, destination, serviceRef, sni, b.localDatacenter, true).
		addClusters(portInfo, destination, sni)
}

func (b *Builder) addClusters(portInfo *servicePortInfo, destination *intermediate.Destination, sni string) *Builder {
	for portName, port := range portInfo.servicePorts {
		if port.GetProtocol() != pbcatalog.Protocol_PROTOCOL_TCP {
			//only implementing L4 at the moment
		} else {
			clusterName := fmt.Sprintf("%s.%s", portName, sni)
			b.addCluster(clusterName, sni, portName, destination.Identities)
		}
	}
	return b
}

func (b *Builder) addRouters(portInfo *servicePortInfo, destination *intermediate.Destination,
	serviceRef *pbresource.Reference, sni, datacenter string, isImplicitDestination bool) *Builder {

	for portName, port := range portInfo.servicePorts {
		statPrefix := DestinationStatPrefix(serviceRef, portName, datacenter)

		if port.GetProtocol() != pbcatalog.Protocol_PROTOCOL_TCP {
			//only implementing L4 at the moment
			continue
		}

		clusterName := fmt.Sprintf("%s.%s", portName, sni)
		var portForRouterMatch *pbcatalog.WorkloadPort
		// router matches based on destination ports should only occur on implicit destinations
		// for explicit, nil will get passed to addRouterWithIPAndPortMatch() which will then
		// exclude the destinationPort match on the listener router.
		if isImplicitDestination {
			portForRouterMatch = port
		}
		b.addRouterWithIPAndPortMatch(clusterName, statPrefix, portForRouterMatch, destination.VirtualIPs)
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

func (b *Builder) addRouterDestination(router *pbproxystate.Router, clusterName, statPrefix string, port *pbcatalog.WorkloadPort) *Builder {
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
	return b
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

func (b *Builder) addCluster(clusterName, sni, portName string, destinationIdentities []*pbresource.Reference) *Builder {
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

	return b
}

func (b *Builder) addEndpointsRef(clusterName string, serviceEndpointsID *pbresource.ID, destinationPort string) *Builder {
	b.proxyStateTemplate.RequiredEndpoints[clusterName] = &pbproxystate.EndpointRef{
		Id:   serviceEndpointsID,
		Port: destinationPort,
	}
	return b
}

func (b *Builder) getLastBuiltListener() *pbproxystate.Listener {
	lastBuiltIndex := len(b.proxyStateTemplate.ProxyState.Listeners) - 1
	return b.proxyStateTemplate.ProxyState.Listeners[lastBuiltIndex]
}

func getServicePortInfo(serviceEndpoints *pbcatalog.ServiceEndpoints) *servicePortInfo {
	spInfo := &servicePortInfo{
		servicePorts: make(map[string]*pbcatalog.WorkloadPort),
	}
	type seenData struct {
		port      *pbcatalog.WorkloadPort
		timesSeen int
	}
	seen := make(map[string]*seenData)
	numberOfEndpointAddresses := 0
	for _, ep := range serviceEndpoints.GetEndpoints() {
		for _, address := range ep.Addresses {
			numberOfEndpointAddresses++
			hasAddressLevelPorts := false
			if len(address.Ports) > 0 {
				hasAddressLevelPorts = true
			}

			// if address has specific ports, add those to the seen array
			for _, portName := range address.Ports {
				// check that it is not service mesh port because we don't
				// want to add that to the service ports map.
				epPort, epOK := ep.Ports[portName]
				if isMeshPort(epPort) {
					continue
				}

				portData, ok := seen[portName]
				if ok {
					portData.timesSeen += 1
				} else {
					if epOK {
						seen[portName] = &seenData{port: epPort, timesSeen: 1}
					}
				}
			}

			// iterate through endpoint ports and set the mesh port
			// as well as all endpoint ports for this workload if there
			// are no specific workload ports.
			for epPortName, epPort := range ep.Ports {
				// look to set mesh port
				if isMeshPort(epPort) {
					spInfo.meshPortName = epPortName
					spInfo.meshPort = epPort
					continue
				}

				// if address specifies a subset, it has already been accounted
				// for in the seen list.
				if hasAddressLevelPorts {
					continue
				}
				// otherwise, add all ports for this endpoint.
				portData, ok := seen[epPortName]
				if ok {
					portData.timesSeen += 1
				} else {
					seen[epPortName] = &seenData{port: epPort, timesSeen: 1}
				}
			}
		}
	}

	for portName, portData := range seen {
		// make sure each port is available to all workloads
		if portData.timesSeen == numberOfEndpointAddresses {
			spInfo.servicePorts[portName] = portData.port
		}
	}
	return spInfo
}
