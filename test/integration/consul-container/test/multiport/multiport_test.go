package multiport

import (
	"context"
	"embed"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
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

	_ = createServices(t, cluster)
	time.Sleep(10 * time.Minute)
	//_, adminPort := clientService.GetAdminAddr()
	//
	//libassert.AssertUpstreamEndpointStatus(t, adminPort, "static-server.default", "HEALTHY", 1)
	//libassert.AssertContainerState(t, clientService, "running")
	//assertHTTPRequestToVirtualAddress(t, clientService, "static-server")
}

// createServices creates the static-client and static-server services with
// transparent proxy enabled. It returns a Service for the static-client.
func createServices(t *testing.T, cluster *libcluster.Cluster) *libcluster.ConsulDataplaneContainer {
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

		// Create a service
		_, err := createAndRegisterStaticServer(t, node, serviceOpts.HTTPPort, serviceOpts.GRPCPort, serviceOpts, nil)
		require.NoError(t, err)

		//libassert.CatalogServiceExists(t, client, "static-server-sidecar-proxy", nil)
		//libassert.CatalogServiceExists(t, client, libservice.StaticServerServiceName, nil)
	}

	{
		node := cluster.Agents[2]
		//client := node.GetClient()

		// Do some trickery to ensure that partial completion is correctly torn
		// down, but successful execution is not.
		var deferClean utils.ResettableDefer
		defer deferClean.Execute()

		// Create Consul Dataplanes
		clientDataplane, err := libcluster.NewConsulDataplane(context.Background(), "static-client-1", "0.0.0.0", 8502, node)
		require.NoError(t, err)
		deferClean.Add(func() {
			_ = clientDataplane.Terminate()
		})

		//libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)
		return clientDataplane
	}
}

func createAndRegisterStaticServer(t *testing.T, node libcluster.Agent, httpPort int,
	grpcPort int, svcOpts *libservice.ServiceOpts,
	customContainerCfg func(testcontainers.ContainerRequest) testcontainers.ContainerRequest,
	containerArgs ...string) (libservice.Service, error) {
	// Do some trickery to ensure that partial completion is correctly torn
	// down, but successful execution is not.
	var deferClean utils.ResettableDefer
	defer deferClean.Execute()

	// Create a service and proxy instance
	serverService, err := libservice.NewExampleService(context.Background(), svcOpts.ID, httpPort, grpcPort, node, containerArgs...)
	if err != nil {
		return nil, err
	}
	deferClean.Add(func() {
		_ = serverService.Terminate()
	})

	// Create Consul Dataplanes
	serverDataplane, err := libcluster.NewConsulDataplane(context.Background(), "static-server-1", "0.0.0.0", 8502, node)
	require.NoError(t, err)
	deferClean.Add(func() {
		_ = serverDataplane.Terminate()
	})

	// disable cleanup functions now that we have an object with a Terminate() function
	deferClean.Reset()

	return serverService, nil
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
