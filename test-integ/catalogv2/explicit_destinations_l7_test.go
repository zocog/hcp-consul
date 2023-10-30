// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package catalogv2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"

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
			case "tcp":
				// we expect 2 clusters, one for each leg of the split
				asserter.UpstreamEndpointStatus(t, svc, v1ClusterPrefix+".", "HEALTHY", 1)
				asserter.UpstreamEndpointStatus(t, svc, v2ClusterPrefix+".", "HEALTHY", 1)

				asserter.TCPServiceProbe(t, svc, u.LocalPort)

				// // Both should be possible.
				// v1Expect := fmt.Sprintf("%s::%s", cluster.Name, v1ID.String())
				// v2Expect := fmt.Sprintf("%s::%s", cluster.Name, v2ID.String())

				// got := make(map[string]int)
				// asserter.FortioFetch2FortioNameCallback(t, svc, u, 100, func(_ *retry.R, name string) {
				// 	got[name]++
				// }, func(r *retry.R) {
				// 	assertTrafficSplit(r, got, map[string]int{v1Expect: 10, v2Expect: 90}, 2)
				// })

			case "grpc":
				// we expect 2 clusters, one for each leg of the split
				asserter.UpstreamEndpointStatus(t, svc, v1ClusterPrefix+".", "HEALTHY", 1)
				asserter.UpstreamEndpointStatus(t, svc, v2ClusterPrefix+".", "HEALTHY", 1)

				makeGRPCCallToBlankspaceApp(t, svc, u.LocalPort)

				// // Both should be possible.
				// v1Expect := fmt.Sprintf("%s::%s", cluster.Name, v1ID.String())
				// v2Expect := fmt.Sprintf("%s::%s", cluster.Name, v2ID.String())

				// got := make(map[string]int)
				// asserter.FortioFetch2FortioNameCallback(t, svc, u, 100, func(_ *retry.R, name string) {
				// 	got[name]++
				// }, func(r *retry.R) {
				// 	assertTrafficSplit(r, got, map[string]int{v1Expect: 10, v2Expect: 90}, 2)
				// })

			case "http":
				// we expect 2 clusters, one for each leg of the split
				asserter.UpstreamEndpointStatus(t, svc, v1ClusterPrefix+".", "HEALTHY", 1)
				asserter.UpstreamEndpointStatus(t, svc, v2ClusterPrefix+".", "HEALTHY", 1)

				asserter.HTTPServiceEchoes(t, svc, u.LocalPort, "")

				// Both should be possible.
				v1Expect := fmt.Sprintf("%s::%s", cluster.Name, v1ID.String())
				v2Expect := fmt.Sprintf("%s::%s", cluster.Name, v2ID.String())

				got := make(map[string]int)
				makeHTTPCallToBlankspaceAppCallback(t, asserter, svc, u, 100, func(_ *retry.R, name string) {
					got[name]++
				}, func(r *retry.R) {
					assertTrafficSplit(r, got, map[string]int{v1Expect: 10, v2Expect: 90}, 2)
				})
			case "http2":
				asserter.UpstreamEndpointStatus(t, svc, v1ClusterPrefix+".", "HEALTHY", 1)

				asserter.HTTPServiceEchoes(t, svc, u.LocalPort, "")

				// Only v1 is possible.
				makeHTTPCallToBlankspaceApp(t, asserter, svc, u, cluster.Name, v1ID)
			default:
				t.Fatalf("unexpected port name: %s", u.PortName)
			}
		})
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

func newBlankspaceServiceWithDefaults(
	cluster string,
	sid topology.ServiceID,
	nodeVersion topology.NodeVersion,
	mut func(s *topology.Service),
) *topology.Service {
	const (
		httpPort  = 8080
		grpcPort  = 8079
		tcpPort   = 8078
		adminPort = 19000
	)
	sid.Normalize()

	svc := &topology.Service{
		ID:             sid,
		Image:          "rboyer/blankspace",
		EnvoyAdminPort: adminPort,
		CheckTCP:       "127.0.0.1:" + strconv.Itoa(httpPort),
		Command: []string{
			"-name", cluster + "::" + sid.String(),
			"-http-port", strconv.Itoa(httpPort),
			"-grpc-port", strconv.Itoa(grpcPort),
			"-tcp-port", strconv.Itoa(tcpPort),
		},
	}

	if nodeVersion == topology.NodeVersionV2 {
		svc.Ports = map[string]*topology.Port{
			"http":  {Number: httpPort, Protocol: "http"},
			"http2": {Number: httpPort, Protocol: "http2"},
			"grpc":  {Number: grpcPort, Protocol: "grpc"},
			"tcp":   {Number: tcpPort, Protocol: "tcp"},
		}
	} else {
		svc.Port = httpPort
	}

	if mut != nil {
		mut(svc)
	}
	return svc
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
			newBlankspaceServiceWithDefaults(
				clusterName,
				newServiceID("static-server-v1"),
				topology.NodeVersionV2,
				func(svc *topology.Service) {
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
			newBlankspaceServiceWithDefaults(
				clusterName,
				newServiceID("static-server-v2"),
				topology.NodeVersionV2,
				func(svc *topology.Service) {
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
			newBlankspaceServiceWithDefaults(
				clusterName,
				newServiceID("static-client"),
				topology.NodeVersionV2,
				func(svc *topology.Service) {
					svc.Upstreams = []*topology.Upstream{
						{
							ID:           newServiceID("static-server"),
							PortName:     "http",
							LocalAddress: "0.0.0.0", // needed for an assertion
							LocalPort:    5000,
						},
						{
							ID:           newServiceID("static-server"),
							PortName:     "http2",
							LocalAddress: "0.0.0.0", // needed for an assertion
							LocalPort:    5001,
						},
						{
							ID:           newServiceID("static-server"),
							PortName:     "grpc",
							LocalAddress: "0.0.0.0", // needed for an assertion
							LocalPort:    5002,
						},
						{
							ID:           newServiceID("static-server"),
							PortName:     "tcp",
							LocalAddress: "0.0.0.0", // needed for an assertion
							LocalPort:    5003,
						},
					}
				},
			),
		},
	}

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
				Protocol:   pbcatalog.Protocol_PROTOCOL_HTTP,
			},
			{
				TargetPort: "http2",
				Protocol:   pbcatalog.Protocol_PROTOCOL_HTTP2,
			},
			{
				TargetPort: "grpc",
				Protocol:   pbcatalog.Protocol_PROTOCOL_GRPC,
			},
			{
				TargetPort: "tcp",
				Protocol:   pbcatalog.Protocol_PROTOCOL_TCP,
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
					Weight: 10,
				},
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v2",
							Tenancy: tenancy,
						},
					},
					Weight: 90,
				},
			},
		}},
	})
	http2ServerRoute := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbmesh.HTTPRouteType,
			Name:    "static-server-http2-route",
			Tenancy: tenancy,
		},
	}, &pbmesh.HTTPRoute{
		ParentRefs: []*pbmesh.ParentReference{{
			Ref: &pbresource.Reference{
				Type:    pbcatalog.ServiceType,
				Name:    "static-server",
				Tenancy: tenancy,
			},
			Port: "http2",
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
	grpcServerRoute := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbmesh.GRPCRouteType,
			Name:    "static-server-grpc-route",
			Tenancy: tenancy,
		},
	}, &pbmesh.GRPCRoute{
		ParentRefs: []*pbmesh.ParentReference{{
			Ref: &pbresource.Reference{
				Type:    pbcatalog.ServiceType,
				Name:    "static-server",
				Tenancy: tenancy,
			},
			Port: "grpc",
		}},
		Rules: []*pbmesh.GRPCRouteRule{{
			BackendRefs: []*pbmesh.GRPCBackendRef{
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v1",
							Tenancy: tenancy,
						},
					},
					Weight: 10,
				},
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v2",
							Tenancy: tenancy,
						},
					},
					Weight: 90,
				},
			},
		}},
	})
	tcpServerRoute := sprawltest.MustSetResourceData(t, &pbresource.Resource{
		Id: &pbresource.ID{
			Type:    pbmesh.TCPRouteType,
			Name:    "static-server-tcp-route",
			Tenancy: tenancy,
		},
	}, &pbmesh.TCPRoute{
		ParentRefs: []*pbmesh.ParentReference{{
			Ref: &pbresource.Reference{
				Type:    pbcatalog.ServiceType,
				Name:    "static-server",
				Tenancy: tenancy,
			},
			Port: "tcp",
		}},
		Rules: []*pbmesh.TCPRouteRule{{
			BackendRefs: []*pbmesh.TCPBackendRef{
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v1",
							Tenancy: tenancy,
						},
					},
					Weight: 10,
				},
				{
					BackendRef: &pbmesh.BackendReference{
						Ref: &pbresource.Reference{
							Type:    pbcatalog.ServiceType,
							Name:    "static-server-v2",
							Tenancy: tenancy,
						},
					},
					Weight: 90,
				},
			},
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
		http2ServerRoute,
		tcpServerRoute,
		grpcServerRoute,
	)
}

func makeHTTPCallToBlankspaceApp(
	t *testing.T,
	a *topoutil.Asserter,
	service *topology.Service,
	upstream *topology.Upstream,
	clusterName string,
	sid topology.ServiceID,
) {
	t.Helper()

	var (
		node   = service.Node
		addr   = fmt.Sprintf("%s:%d", node.LocalAddress(), service.PortOrDefault(upstream.PortName))
		client = a.MustGetHTTPClient(t, node.Cluster)
	)

	retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: time.Millisecond * 500}, t, func(r *retry.R) {
		body, res := blankspaceFetchUpstream(r, client, addr, upstream, "/")

		require.Equal(r, http.StatusOK, res.StatusCode)

		var v struct {
			Name string
		}
		require.NoError(r, json.Unmarshal(body, &v))
		require.Equal(r, fmt.Sprintf("%s::%s", clusterName, sid.String()), v.Name)
	})
}

func makeHTTPCallToBlankspaceAppCallback(
	t *testing.T,
	a *topoutil.Asserter,
	service *topology.Service,
	upstream *topology.Upstream,
	count int,
	attemptFn func(r *retry.R, remoteName string),
	checkFn func(r *retry.R),
) {
	t.Helper()

	var (
		node   = service.Node
		addr   = fmt.Sprintf("%s:%d", node.LocalAddress(), service.PortOrDefault(upstream.PortName))
		client = a.MustGetHTTPClient(t, node.Cluster)
	)

	retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: time.Millisecond * 500}, t, func(r *retry.R) {
		for i := 0; i < count; i++ {
			body, res := blankspaceFetchUpstream(r, client, addr, upstream, "/")

			require.Equal(r, http.StatusOK, res.StatusCode)

			var v struct {
				Name string
			}
			require.NoError(r, json.Unmarshal(body, &v))
			attemptFn(r, v.Name)
		}
		checkFn(r)
	})
}

type testingT interface {
	require.TestingT
	Helper()
}

// does a fortio /fetch2 to the given fortio service, targetting the given upstream. Returns
// the body, and response with response.Body already Closed.
//
// We treat 400, 503, and 504s as retryable errors
func blankspaceFetchUpstream(
	t testingT,
	client *http.Client,
	addr string,
	upstream *topology.Upstream,
	path string,
) (body []byte, res *http.Response) {
	t.Helper()

	var actualURL string
	if upstream.Implied {
		actualURL = fmt.Sprintf("http://%s--%s--%s.virtual.consul:%d/%s",
			upstream.ID.Name,
			upstream.ID.Namespace,
			upstream.ID.Partition,
			upstream.VirtualPort,
			path,
		)
	} else {
		actualURL = fmt.Sprintf("http://localhost:%d/%s", upstream.LocalPort, path)
	}

	url := fmt.Sprintf("http://%s/fetch?url=%s", addr,
		url.QueryEscape(actualURL),
	)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	res, err = client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	// not sure when these happen, suspect it's when the mesh gateway in the peer is not yet ready
	require.NotEqual(t, http.StatusServiceUnavailable, res.StatusCode)
	require.NotEqual(t, http.StatusGatewayTimeout, res.StatusCode)
	// not sure when this happens, suspect it's when envoy hasn't configured the local upstream yet
	require.NotEqual(t, http.StatusBadRequest, res.StatusCode)
	body, err = io.ReadAll(res.Body)
	require.NoError(t, err)

	return body, res
}

func makeGRPCCallToBlankspaceApp(
	t *testing.T,
	service *topology.Service,
	port int,
) {
	t.Helper()
	require.True(t, port > 0)

	node := service.Node

	// We can't use the forward proxy for gRPC yet, so use the exposed port on localhost instead.
	exposedPort := node.ExposedPort(port)
	require.True(t, exposedPort > 0)

	addr := fmt.Sprintf("%s:%d", "127.0.0.1", exposedPort)

	failer := func() *retry.Timer {
		return &retry.Timer{Timeout: 30 * time.Second, Wait: 500 * time.Millisecond}
	}

	retry.RunWith(failer(), t, func(r *retry.R) {
		err := sendFortioGRPCPing(context.Background(), addr)
		require.NoError(r, err)
	})
}

func sendFortioGRPCPing(ctx context.Context, serverAddr string) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	desc := dynamicpbkj

	// conn.Invoke(ctx,  "/blankspace.Server/Describe", in, out, opts...)
	// Invoke(ctx context.Context, method string, args any, reply any, opts ...CallOption) error
	client := grpc_reflection_v1.NewServerReflectionClient(conn)
	dynamicpb.Message

	// func testFileByFilenameTransitiveClosure(t *testing.T, stream v1reflectiongrpc.ServerReflection_ServerReflectionInfoClient, expectClosure bool) {
	// filename := "reflection/grpc_testing/proto2_ext2.proto"
	// if err := stream.Send(&v1reflectionpb.ServerReflectionRequest{
	// 	MessageRequest: &v1reflectionpb.ServerReflectionRequest_FileByFilename{
	// 		FileByFilename: filename,
	// 	},
	// }); err != nil {
	// 	t.Fatalf("failed to send request: %v", err)
	// }
	// r, err := stream.Recv()
	// if err != nil {
	// 	// io.EOF is not ok.
	// 	t.Fatalf("failed to recv response: %v", err)
	// }
	// switch r.MessageResponse.(type) {
	// case *v1reflectionpb.ServerReflectionResponse_FileDescriptorResponse:
	// 	if !reflect.DeepEqual(r.GetFileDescriptorResponse().FileDescriptorProto[0], fdProto2Ext2Byte) {
	// 		t.Errorf("FileByFilename(%v)\nreceived: %q,\nwant: %q", filename, r.GetFileDescriptorResponse().FileDescriptorProto[0], fdProto2Ext2Byte)
	// 	}
	// 	if expectClosure {
	// 		if len(r.GetFileDescriptorResponse().FileDescriptorProto) != 2 {
	// 			t.Errorf("FileByFilename(%v) returned %v file descriptors, expected 2", filename, len(r.GetFileDescriptorResponse().FileDescriptorProto))
	// 		} else if !reflect.DeepEqual(r.GetFileDescriptorResponse().FileDescriptorProto[1], fdProto2Byte) {
	// 			t.Errorf("FileByFilename(%v)\nreceived: %q,\nwant: %q", filename, r.GetFileDescriptorResponse().FileDescriptorProto[1], fdProto2Byte)
	// 		}
	// 	} else if len(r.GetFileDescriptorResponse().FileDescriptorProto) != 1 {
	// 		t.Errorf("FileByFilename(%v) returned %v file descriptors, expected 1", filename, len(r.GetFileDescriptorResponse().FileDescriptorProto))
	// 	}
	// default:
	// 	t.Errorf("FileByFilename(%v) = %v, want type <ServerReflectionResponse_FileDescriptorResponse>", filename, r.MessageResponse)
	// }

	stream, err := client.ServerReflectionInfo(ctx, )
	if err != nil {
		return fmt.Errorf("grpc error from Ping: %w", err)
	}

	if err := stream.Send(&grpc_reflection_v1.ServerReflectionRequest{})

	return nil
}
