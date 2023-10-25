// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package catalogv2

import (
	"fmt"
	"testing"

	pbauth "github.com/hashicorp/consul/proto-public/pbauth/v2beta1"
	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v2beta1"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v2beta1"
	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
	"github.com/hashicorp/consul/testing/deployer/sprawl/sprawltest"
	"github.com/hashicorp/consul/testing/deployer/topology"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/test-integ/topoutil"
)

func TestSplitterFeaturesL7ExplicitDestinations(t *testing.T) {
	cfg := testSplitterFeaturesL7ExplicitDestinationsCreator{}.NewConfig(t)

	sp := sprawltest.Launch(t, cfg)

	var (
		asserter = topoutil.NewAsserter(sp)

		topo    = sp.Topology()
		cluster = topo.Clusters["dc1"]

		ships = topo.ComputeRelationships()
	)

	clientV2 := sp.ResourceServiceClientForCluster(cluster.Name)

	t.Log(topology.RenderRelationships(ships))

	// Make sure things are in v2.
	libassert.CatalogV2ServiceHasEndpointCount(t, clientV2, "static-client", nil, 1)
	libassert.CatalogV2ServiceHasEndpointCount(t, clientV2, "static-server-v1", nil, 1)
	libassert.CatalogV2ServiceHasEndpointCount(t, clientV2, "static-server-v2", nil, 1)
	libassert.CatalogV2ServiceHasEndpointCount(t, clientV2, "static-server", nil, 0)

	// Check relationships
	for _, ship := range ships {
		t.Run("relationship: "+ship.String(), func(t *testing.T) {
			var (
				svc = ship.Caller
				u   = ship.Upstream
			)

			v1ID := ship.Upstream.ID
			v1ID.Name = "static-server-v1"
			v1ClusterPrefix := clusterPrefix(u.PortName, v1ID, u.Cluster)

			v2ID := ship.Upstream.ID
			v2ID.Name = "static-server-v2"
			v2ClusterPrefix := clusterPrefix(u.PortName, v2ID, u.Cluster)

			switch u.PortName {
			case "http":
				// we expect 2 clusters, one for each leg of the split
				asserter.UpstreamEndpointStatus(t, svc, v1ClusterPrefix+".", "HEALTHY", 1)
				asserter.UpstreamEndpointStatus(t, svc, v2ClusterPrefix+".", "HEALTHY", 1)

				asserter.HTTPServiceEchoes(t, svc, u.LocalPort, "")

				// Both should be possible.
				v1Expect := fmt.Sprintf("%s::%s", cluster.Name, v1ID.String())
				v2Expect := fmt.Sprintf("%s::%s", cluster.Name, v2ID.String())

				got := make(map[string]int)
				asserter.FortioFetch2FortioNameCallback(t, svc, u, 100, func(_ *retry.R, name string) {
					got[name]++
				}, func(r *retry.R) {
					assertTrafficSplit(r, got, map[string]int{
						v1Expect: 50,
						v2Expect: 50,
					}, 2)
					// require.Len(r, got, 2)
					// v1Count, v1Exist := got[v1Expect]
					// v2Count, v2Exist := got[v2Expect]
					// require.True(r, v1Exist)
					// require.True(r, v2Exist)

					// diff := v1Count - v2Count
					// if diff < 0 {
					// 	diff = -diff
					// }
					// pct := diff // 100 * abs(a-b) / 100

					// // Acceptable fudge error is +/-2
					// require.True(r, pct <= 2, "split was not 50/50: v1=%d v2=%d", v1Count, v2Count)
				})
			case "http-alt":
				asserter.UpstreamEndpointStatus(t, svc, v1ClusterPrefix+".", "HEALTHY", 1)

				asserter.HTTPServiceEchoes(t, svc, u.LocalPort, "")

				// Only v1 is possible.
				asserter.FortioFetch2FortioName(t, svc, u, cluster.Name, v1ID)
			default:
				t.Fatalf("unexpected port name: %s", u.PortName)
			}
		})
	}
}

func assertTrafficSplit(t require.TestingT, nameCounts map[string]int, expect map[string]int, epsilon int) {
	require.Len(t, nameCounts, len(expect))
	for name, expectCount := range expect {
		gotCount, ok := nameCounts[name]
		require.True(t, ok)
		require.InEpsilon(t, expectCount, gotCount, float64(epsilon),
			"expected %q side of split to have %d requests not %d (e=%d)",
			name, expectCount, gotCount, epsilon,
		)
	}
}

type testSplitterFeaturesL7ExplicitDestinationsCreator struct{}

func (c testSplitterFeaturesL7ExplicitDestinationsCreator) NewConfig(t *testing.T) *topology.Config {
	const clusterName = "dc1"

	servers := topoutil.NewTopologyServerSet(clusterName+"-server", 1 /*3*/, []string{clusterName, "wan"}, nil)

	cluster := &topology.Cluster{
		Enterprise: utils.IsEnterprise(),
		Name:       clusterName,
		Nodes:      servers,
	}

	lastNode := 0
	nodeName := func() string {
		lastNode++
		return fmt.Sprintf("%s-box%d", clusterName, lastNode)
	}

	c.topologyConfigAddNodes(t, cluster, nodeName, "default", "default")
	if cluster.Enterprise {
		c.topologyConfigAddNodes(t, cluster, nodeName, "part1", "default")
		c.topologyConfigAddNodes(t, cluster, nodeName, "part1", "nsa")
		c.topologyConfigAddNodes(t, cluster, nodeName, "default", "nsa")
	}

	return &topology.Config{
		Images: topoutil.TargetImages(),
		Networks: []*topology.Network{
			{Name: clusterName},
			{Name: "wan", Type: "wan"},
		},
		Clusters: []*topology.Cluster{
			cluster,
		},
	}
}

func (c testSplitterFeaturesL7ExplicitDestinationsCreator) topologyConfigAddNodes(
	t *testing.T,
	cluster *topology.Cluster,
	nodeName func() string,
	partition,
	namespace string,
) {
	clusterName := cluster.Name

	newServiceID := func(name string) topology.ServiceID {
		return topology.ServiceID{
			Partition: partition,
			Namespace: namespace,
			Name:      name,
		}
	}

	tenancy := &pbresource.Tenancy{
		Partition: partition,
		Namespace: namespace,
		PeerName:  "local",
	}

	v1ServerNode := &topology.Node{
		Kind:      topology.NodeKindDataplane,
		Version:   topology.NodeVersionV2,
		Partition: partition,
		Name:      nodeName(),
		Services: []*topology.Service{
			topoutil.NewFortioServiceWithDefaults(
				clusterName,
				newServiceID("static-server-v1"),
				topology.NodeVersionV2,
				func(svc *topology.Service) {
					delete(svc.Ports, "grpc") // v2 mode turns this on, so turn it off
					svc.Meta = map[string]string{
						"version": "v1",
					}
					// svc.V2Services = []string{"static-server"}
					svc.WorkloadIdentity = "static-server-v1"
				},
			),
		},
	}
	v2ServerNode := &topology.Node{
		Kind:      topology.NodeKindDataplane,
		Version:   topology.NodeVersionV2,
		Partition: partition,
		Name:      nodeName(),
		Services: []*topology.Service{
			topoutil.NewFortioServiceWithDefaults(
				clusterName,
				newServiceID("static-server-v2"),
				topology.NodeVersionV2,
				func(svc *topology.Service) {
					delete(svc.Ports, "grpc") // v2 mode turns this on, so turn it off
					svc.Meta = map[string]string{
						"version": "v2",
					}
					// svc.V2Services = []string{"static-server"}
					svc.WorkloadIdentity = "static-server-v2"
				},
			),
		},
	}
	clientNode := &topology.Node{
		Kind:      topology.NodeKindDataplane,
		Version:   topology.NodeVersionV2,
		Partition: partition,
		Name:      nodeName(),
		Services: []*topology.Service{
			topoutil.NewFortioServiceWithDefaults(
				clusterName,
				newServiceID("static-client"),
				topology.NodeVersionV2,
				func(svc *topology.Service) {
					delete(svc.Ports, "grpc") // v2 mode turns this on, so turn it off
					svc.Upstreams = []*topology.Upstream{
						{
							ID:           newServiceID("static-server"),
							PortName:     "http",
							LocalAddress: "0.0.0.0", // needed for an assertion
							LocalPort:    5000,
						},
						{
							ID:           newServiceID("static-server"),
							PortName:     "http-alt",
							LocalAddress: "0.0.0.0", // needed for an assertion
							LocalPort:    5001,
						},
					}
				},
			),
		},
	}

	// static-server-v2.default.dc1.internal.f6823a6f-84f8-eec5-22cb-849f78d0bdd8.consul
	v1TrafficPerms := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbauth.TrafficPermissionsType,
			Name:    "static-server-v1-perms",
			Tenancy: tenancy,
		},
	}, &pbauth.TrafficPermissions{
		Destination: &pbauth.Destination{
			IdentityName: "static-server-v1",
		},
		Action: pbauth.Action_ACTION_ALLOW,
		Permissions: []*pbauth.Permission{{
			Sources: []*pbauth.Source{{
				IdentityName: "static-client",
				Namespace:    namespace,
			}},
		}},
	})
	v2TrafficPerms := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbauth.TrafficPermissionsType,
			Name:    "static-server-v2-perms",
			Tenancy: tenancy,
		},
	}, &pbauth.TrafficPermissions{
		Destination: &pbauth.Destination{
			IdentityName: "static-server-v2",
		},
		Action: pbauth.Action_ACTION_ALLOW,
		Permissions: []*pbauth.Permission{{
			Sources: []*pbauth.Source{{
				IdentityName: "static-client",
				Namespace:    namespace,
			}},
		}},
	})

	staticServerService := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbcatalog.ServiceType,
			Name:    "static-server",
			Tenancy: tenancy,
		},
	}, &pbcatalog.Service{
		Workloads: &pbcatalog.WorkloadSelector{
			// This will result in a 50/50 uncontrolled split.
			Prefixes: []string{"static-server-"},
		},
		Ports: []*pbcatalog.ServicePort{
			{
				TargetPort: "http",
				Protocol:   pbcatalog.Protocol_PROTOCOL_TCP, // TODO
			},
			{
				TargetPort: "http-alt",
				Protocol:   pbcatalog.Protocol_PROTOCOL_TCP, // TODO
			},
			{
				TargetPort: "mesh",
				Protocol:   pbcatalog.Protocol_PROTOCOL_MESH,
			},
		},
	})

	httpServerRoute := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbmesh.HTTPRouteType,
			Name:    "static-server-http-route",
			Tenancy: tenancy,
		},
	}, &pbmesh.HTTPRoute{
		ParentRefs: []*pbmesh.ParentReference{{
			Ref: &pbresource.Reference{
				Type:    pbcatalog.ServiceType,
				Name:    "static-server",
				Tenancy: tenancy,
			},
			Port: "http",
		}},
		Rules: []*pbmesh.HTTPRouteRule{{
			BackendRefs: []*pbmesh.HTTPBackendRef{
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v1",
							Tenancy: tenancy,
						},
					},
					Weight: 50,
				},
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v2",
							Tenancy: tenancy,
						},
					},
					Weight: 50,
				},
			},
		}},
	})
	httpAltServerRoute := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbmesh.HTTPRouteType,
			Name:    "static-server-http-alt-route",
			Tenancy: tenancy,
		},
	}, &pbmesh.HTTPRoute{
		ParentRefs: []*pbmesh.ParentReference{{
			Ref: &pbresource.Reference{
				Type:    pbcatalog.ServiceType,
				Name:    "static-server",
				Tenancy: tenancy,
			},
			Port: "http-alt",
		}},
		Rules: []*pbmesh.HTTPRouteRule{{
			BackendRefs: []*pbmesh.HTTPBackendRef{{
				BackendRef: &pbmesh.BackendReference{
					Ref: &pbresource.Reference{
						Type:    pbcatalog.ServiceType,
						Name:    "static-server-v1",
						Tenancy: tenancy,
					},
				},
			}},
		}},
	})

	cluster.Nodes = append(cluster.Nodes,
		clientNode,
		v1ServerNode,
		v2ServerNode,
	)

	cluster.InitialResources = append(cluster.InitialResources,
		staticServerService,
		v1TrafficPerms,
		v2TrafficPerms,
		httpServerRoute,
		httpAltServerRoute,
	)
}
