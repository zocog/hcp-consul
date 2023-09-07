package multiport

import (
	"context"
	"embed"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	rtest "github.com/hashicorp/consul/internal/resource/resourcetest"
	"github.com/hashicorp/consul/proto-public/pbresource"
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

	createServices(t, cluster)
	time.Sleep(10 * time.Minute)
	//_, adminPort := clientService.GetAdminAddr()
	//
	//libassert.AssertUpstreamEndpointStatus(t, adminPort, "static-server.default", "HEALTHY", 1)
	//libassert.AssertContainerState(t, clientService, "running")
	//assertHTTPRequestToVirtualAddress(t, clientService, "static-server")
}

// createServices creates the static-client and static-server services with
// transparent proxy enabled. It returns a Service for the static-client.
func createServices(t *testing.T, cluster *libcluster.Cluster) {
	{
		node := cluster.Agents[1]
		//client := node.GetClient()

		// Create a service and dataplane
		_, err := createServiceAndDataplane(t, node, "static-server-1", "static-server", 8080, 8079)
		require.NoError(t, err)

		//libassert.CatalogServiceExists(t, client, "static-server-sidecar-proxy", nil)
		//libassert.CatalogServiceExists(t, client, libservice.StaticServerServiceName, nil)
	}

	{
		node := cluster.Agents[2]
		// Create a service and dataplane
		_, err := createServiceAndDataplane(t, node, "static-client-1", "static-client", 8080, 8079)
		require.NoError(t, err)

		//libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)
	}
}

func createServiceAndDataplane(t *testing.T, node libcluster.Agent, proxyID, serviceName string, httpPort, grpcPort int) (libservice.Service, error) {
	// Do some trickery to ensure that partial completion is correctly torn
	// down, but successful execution is not.
	var deferClean utils.ResettableDefer
	defer deferClean.Execute()

	// Create a service and proxy instance
	svc, err := libservice.NewExampleService(context.Background(), serviceName, httpPort, grpcPort, node)
	if err != nil {
		return nil, err
	}
	deferClean.Add(func() {
		_ = svc.Terminate()
	})

	// Create Consul Dataplane
	dp, err := libcluster.NewConsulDataplane(context.Background(), proxyID, "0.0.0.0", 8502, node)
	require.NoError(t, err)
	deferClean.Add(func() {
		_ = dp.Terminate()
	})

	// disable cleanup functions now that we have an object with a Terminate() function
	deferClean.Reset()

	return svc, nil
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
