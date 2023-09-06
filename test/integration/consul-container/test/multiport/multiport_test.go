package multiport

import (
	"embed"
	rtest "github.com/hashicorp/consul/internal/resource/resourcetest"
	"github.com/hashicorp/consul/proto-public/pbresource"
	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/sdk/testutil/retry"
	libcluster "github.com/hashicorp/consul/test/integration/consul-container/libs/cluster"
	libservice "github.com/hashicorp/consul/test/integration/consul-container/libs/service"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/topology"
)

var (
	//go:embed integration_test_data
	testData embed.FS

	requestRetryTimer = &retry.Timer{Timeout: 120 * time.Second, Wait: 500 * time.Millisecond}
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

	cluster := createCluster(t, 2) // 2 client agent pods
	followers, err := cluster.Followers()
	require.NoError(t, err)
	client := pbresource.NewResourceServiceClient(followers[0].GetGRPCConn())
	resourceClient := rtest.NewClient(client)

	createServicesWorkloadsAndNodes(t, resourceClient, cluster)
	clientService := createServices(t, cluster)
	_, adminPort := clientService.GetAdminAddr()

	libassert.AssertUpstreamEndpointStatus(t, adminPort, "static-server.default", "HEALTHY", 1)
	libassert.AssertContainerState(t, clientService, "running")
	//assertHTTPRequestToVirtualAddress(t, clientService, "static-server")
}

// createServices creates the static-client and static-server services with
// transparent proxy enabled. It returns a Service for the static-client.
func createServices(t *testing.T, cluster *libcluster.Cluster) libservice.Service {
	{
		node := cluster.Agents[1]
		client := node.GetClient()
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
		_, _, err := libservice.CreateAndRegisterStaticServerAndSidecar(node, serviceOpts)
		require.NoError(t, err)

		libassert.CatalogServiceExists(t, client, "static-server-sidecar-proxy", nil)
		libassert.CatalogServiceExists(t, client, libservice.StaticServerServiceName, nil)
	}

	{
		node := cluster.Agents[2]
		client := node.GetClient()

		// Create a client proxy instance with the server as an upstream
		clientConnectProxy, err := libservice.CreateAndRegisterStaticClientSidecar(node, "", false, true, nil)
		require.NoError(t, err)

		libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)
		return clientConnectProxy
	}
}

func createServicesWorkloadsAndNodes(t *testing.T, resourceClient *rtest.Client, cluster *libcluster.Cluster) {
	resources := rtest.ParseResourcesFromFilesystem(t, testData, "integration_test_data/catalog")
	resourceClient.PublishResources(t, resources)
}

func createCluster(t *testing.T, numClients int) *libcluster.Cluster {
	cluster, _, _ := topology.NewCluster(t, &topology.ClusterConfig{
		NumServers:                3,
		NumClients:                numClients,
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

//// createStaticServerAndSidecar launches static-server containers.
//func createStaticServerAndSidecar(t testing.T, node libcluster.Agent, svc *libservice.ServiceOpts) (libservice.Service, libservice.Service, error) {
//	node := cluster.Agents[1]
//	client := node.GetClient()
//	// Create a service and proxy instance
//	serviceOpts := &libservice.ServiceOpts{
//		Name:     libservice.StaticServerServiceName,
//		ID:       "static-server",
//		HTTPPort: 8080,
//		GRPCPort: 8079,
//		Connect: libservice.SidecarService{
//			Proxy: libservice.ConnectProxy{
//				Mode: "transparent",
//			},
//		},
//	}
//
//	// Create a service and proxy instance
//	_, _, err := libservice.CreateAndRegisterStaticServerAndSidecar(node, serviceOpts)
//	require.NoError(t, err)
//
//	return serverService, serverConnectProxy, nil
//}

func createStaticClient(t *testing.T, cluster *libcluster.Cluster) libservice.Service {
	// Do some trickery to ensure that partial completion is correctly torn
	// down, but successful execution is not.
	var deferClean utils.ResettableDefer
	defer deferClean.Execute()

	node := cluster.Agents[2]
	//client := node.GetClient()

	// Create a client proxy instance with the server as an upstream
	clientConnectProxy, err := libservice.CreateAndRegisterStaticClientSidecar(node, "", false, true, nil)
	require.NoError(t, err)

	//libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)
	return clientConnectProxy

}
