package multiport

import (
	"context"
	"embed"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"testing"

	rtest "github.com/hashicorp/consul/internal/resource/resourcetest"
	"github.com/hashicorp/consul/proto-public/pbresource"
	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
	libcluster "github.com/hashicorp/consul/test/integration/consul-container/libs/cluster"
	libservice "github.com/hashicorp/consul/test/integration/consul-container/libs/service"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/topology"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
)

var (
	//go:embed integration_test_data
	testData embed.FS
)

// TestTProxyService makes sure two services in the same datacenter have connectivity
// with transparent proxy enabled.
//
// Steps:
//   - Create a single server cluster.
//   - Create the example static-server and sidecar containers, then register them both with Consul
//   - Create an example static-client sidecar, then register both the service and sidecar with Consul
//   - Make sure a request from static-client to the virtual address (<svc>.virtual.consul) returns a
//     response from the upstream.
func TestTProxyMultiportService(t *testing.T) {
	t.Parallel()

	cluster := createCluster(t) // 2 client agent pods
	followers, err := cluster.Followers()
	require.NoError(t, err)
	client := pbresource.NewResourceServiceClient(followers[0].GetGRPCConn())
	resourceClient := rtest.NewClient(client)

	createServicesWorkloadsAndNodes(t, resourceClient, cluster)
	clientService := createServices(t, cluster)
	_, adminPort := clientService.GetAdminAddr()
	//
	libassert.AssertUpstreamEndpointStatus(t, adminPort, "static-server.default", "HEALTHY", 1)
	//libassert.AssertContainerState(t, clientService, "running")
	//assertHTTPRequestToVirtualAddress(t, clientService, "static-server")
}

// createServices creates the static-client and static-server services with
// transparent proxy enabled. It returns a Service for the static-client.
func createServices(t *testing.T, cluster *libcluster.Cluster) libservice.Service {
	{
		node := cluster.Agents[1]
		//client := node.GetClient()
		// Create a service and proxy instance
		serviceOpts := &libservice.ServiceOpts{
			Name:     libservice.StaticServerServiceName,
			ID:       "static-server",
			HTTPPort: 8080,
			GRPCPort: 8079,
			Connect: libservice.SidecarService{
				Proxy: libservice.ConnectProxy{
					Mode: "transparent",
				},
			},
		}

		// Create a service and proxy instance
		_, _, err := createAndRegisterStaticServerAndSidecar(t, node, serviceOpts.HTTPPort, serviceOpts.GRPCPort, serviceOpts, nil)
		require.NoError(t, err)

		//libassert.CatalogServiceExists(t, client, "static-server-sidecar-proxy", nil)
		//libassert.CatalogServiceExists(t, client, libservice.StaticServerServiceName, nil)
	}

	{
		node := cluster.Agents[2]
		//client := node.GetClient()

		// Create a client proxy instance with the server as an upstream
		clientConnectProxy, err := createAndRegisterStaticClientSidecar(node, "", false, true, nil)
		require.NoError(t, err)

		// Create Consul Dataplane
		_, err = libcluster.NewConsulDataplane(context.Background(), "static-client-1", "0.0.0.0", 8502, node)
		require.NoError(t, err)

		//libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)
		return clientConnectProxy
	}
}

func createAndRegisterStaticClientSidecar(
	node libcluster.Agent,
	peerName string,
	localMeshGateway bool,
	enableTProxy bool,
	serviceOpts *libservice.ServiceOpts,
) (*libservice.ConnectContainer, error) {
	// Do some trickery to ensure that partial completion is correctly torn
	// down, but successful execution is not.
	var deferClean utils.ResettableDefer
	defer deferClean.Execute()

	//var proxy *api.AgentServiceConnectProxyConfig
	//if enableTProxy {
	//	proxy = &api.AgentServiceConnectProxyConfig{
	//		Mode: "transparent",
	//	}
	//} else {
	//	mgwMode := api.MeshGatewayModeRemote
	//	if localMeshGateway {
	//		mgwMode = api.MeshGatewayModeLocal
	//	}
	//	proxy = &api.AgentServiceConnectProxyConfig{
	//		Upstreams: []api.Upstream{{
	//			DestinationName:  libservice.StaticServerServiceName,
	//			DestinationPeer:  peerName,
	//			LocalBindAddress: "0.0.0.0",
	//			LocalBindPort:    libcluster.ServiceUpstreamLocalBindPort,
	//			MeshGateway: api.MeshGatewayConfig{
	//				Mode: mgwMode,
	//			},
	//		}},
	//	}
	//}

	//// Register the static-client service and sidecar first to prevent race with sidecar
	//// trying to get xDS before it's ready
	//req := &api.AgentServiceRegistration{
	//	Name: libservice.StaticClientServiceName,
	//	Port: 8080,
	//	Connect: &api.AgentServiceConnect{
	//		SidecarService: &api.AgentServiceRegistration{
	//			Proxy: proxy,
	//		},
	//	},
	//}

	//// Set relevant fields for static client if opts are provided
	//if serviceOpts != nil {
	//	if serviceOpts.Connect.Proxy.Mode != "" {
	//		return nil, fmt.Errorf("this helper does not support directly setting connect proxy mode; use enableTProxy and/or localMeshGateway instead")
	//	}
	//	// These options are defaulted above, so only set them as overrides
	//	if serviceOpts.Name != "" {
	//		req.Name = serviceOpts.Name
	//	}
	//	if serviceOpts.HTTPPort != 0 {
	//		req.Port = serviceOpts.HTTPPort
	//	}
	//	if serviceOpts.Connect.Port != 0 {
	//		req.Connect.SidecarService.Port = serviceOpts.Connect.Port
	//	}
	//	req.Meta = serviceOpts.Meta
	//	req.Namespace = serviceOpts.Namespace
	//	req.Partition = serviceOpts.Partition
	//	req.Locality = serviceOpts.Locality
	//}
	//
	//if err := node.GetClient().Agent().ServiceRegister(req); err != nil {
	//	return nil, err
	//}

	// Create a service and proxy instance
	sidecarCfg := libservice.SidecarConfig{
		Name:         fmt.Sprintf("%s-sidecar", libservice.StaticClientServiceName),
		ServiceID:    libservice.StaticClientServiceName,
		EnableTProxy: enableTProxy,
	}

	clientConnectProxy, err := libservice.NewConnectService(context.Background(),
		sidecarCfg, []int{libcluster.ServiceUpstreamLocalBindPort}, node, nil)
	if err != nil {
		return nil, err
	}
	deferClean.Add(func() {
		_ = clientConnectProxy.Terminate()
	})

	// disable cleanup functions now that we have an object with a Terminate() function
	deferClean.Reset()

	return clientConnectProxy, nil
}

func createAndRegisterStaticServerAndSidecar(t *testing.T, node libcluster.Agent, httpPort int,
	grpcPort int, svcOpts *libservice.ServiceOpts,
	customContainerCfg func(testcontainers.ContainerRequest) testcontainers.ContainerRequest,
	containerArgs ...string) (libservice.Service, libservice.Service, error) {
	// Do some trickery to ensure that partial completion is correctly torn
	// down, but successful execution is not.
	var deferClean utils.ResettableDefer
	defer deferClean.Execute()

	// Create a service and proxy instance
	serverService, err := libservice.NewExampleService(context.Background(), svcOpts.ID, httpPort, grpcPort, node, containerArgs...)
	if err != nil {
		return nil, nil, err
	}
	deferClean.Add(func() {
		_ = serverService.Terminate()
	})
	sidecarCfg := libservice.SidecarConfig{
		Name:         fmt.Sprintf("%s-sidecar", svcOpts.ID),
		ServiceID:    svcOpts.ID,
		Namespace:    svcOpts.Namespace,
		Partition:    svcOpts.Partition,
		EnableTProxy: true,
	}
	serverConnectProxy, err := libservice.NewConnectService(context.Background(), sidecarCfg, []int{svcOpts.HTTPPort}, node, customContainerCfg) // bindPort not used
	if err != nil {
		return nil, nil, err
	}

	deferClean.Add(func() {
		_ = serverConnectProxy.Terminate()
	})

	// disable cleanup functions now that we have an object with a Terminate() function
	deferClean.Reset()

	return serverService, serverConnectProxy, nil
}

func createServicesWorkloadsAndNodes(t *testing.T, resourceClient *rtest.Client, cluster *libcluster.Cluster) {
	resources := rtest.ParseResourcesFromFilesystem(t, testData, "integration_test_data/catalog")
	resourceClient.PublishResources(t, resources)
}

func createCluster(t *testing.T) *libcluster.Cluster {
	cluster, _, _ := topology.NewCluster(t, &topology.ClusterConfig{
		NumServers:                3,
		ApplyDefaultProxySettings: true,
		BuildOpts: &libcluster.BuildOptions{
			Datacenter:             "dc1",
			InjectAutoEncryption:   true,
			InjectGossipEncryption: true,
			AllowHTTPAnyway:        true,
		},
		Cmd: `-hcl=experiments=["resource-apis"] log_level="TRACE"`,
	})

	return cluster
}
