package multiport

import (
	"context"
	"embed"
	"fmt"
	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	rtest "github.com/hashicorp/consul/internal/resource/resourcetest"
	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	libcluster "github.com/hashicorp/consul/test/integration/consul-container/libs/cluster"
	libservice "github.com/hashicorp/consul/test/integration/consul-container/libs/service"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/topology"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
)

var (
	//go:embed integration_test_data
	testData          embed.FS
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

	cluster := createCluster(t) // 2 client agent pods
	followers, err := cluster.Followers()
	require.NoError(t, err)
	client := pbresource.NewResourceServiceClient(followers[0].GetGRPCConn())
	resourceClient := rtest.NewClient(client)

	createServicesWorkloadsAndNodes(t, resourceClient, cluster)

	clientSvc := createServices(t, cluster)
	//_, adminPort := clientSvc.GetAdminAddr()
	//
	//libassert.AssertUpstreamEndpointStatus(t, adminPort, "static-server.default", "HEALTHY", 1)
	libassert.AssertContainerState(t, clientSvc, "running")
	assertHTTPRequestToVirtualAddress(t, clientSvc, "static-server")
}

// createServices creates the static-client and static-server services with
// transparent proxy enabled. It returns a Service for the static-client.
func createServices(t *testing.T, cluster *libcluster.Cluster) libservice.Service {
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
		clientService, err := createServiceAndDataplane(t, node, "static-client-1", "static-client", 8080, 8079)
		require.NoError(t, err)

		//libassert.CatalogServiceExists(t, client, "static-client-sidecar-proxy", nil)
		return clientService
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

// assertHTTPRequestToVirtualAddress checks that a request to the
// static-server's virtual address succeeds by running curl in the given
// `clientService` container.
//
// This assumes the destination service is running Fortio. The request is made
// to `<serverName>.virtual.consul/debug?env=dump` and this checks that
// `FORTIO_NAME=<serverName>` is contained in the response.
func assertHTTPRequestToVirtualAddress(t *testing.T, clientService libservice.Service, serverName string) {
	virtualHostname := fmt.Sprintf("%s.virtual.consul", serverName)

	retry.RunWith(requestRetryTimer, t, func(r *retry.R) {
		// Test that we can make a request to the virtual ip to reach the upstream.
		//
		// NOTE(pglass): This uses a workaround for DNS because I had trouble modifying
		// /etc/resolv.conf. There is a --dns option to docker run, but it
		// didn't seem to be exposed via testcontainers. I'm not sure if it would
		// do what I want. In any case, Docker sets up /etc/resolv.conf for certain
		// functionality so it seems better to leave DNS alone.
		//
		// But, that means DNS queries aren't redirected to Consul out of the box.
		// As a workaround, we `dig @localhost:53` which is iptables-redirected to
		// localhost:8600 where the Consul client responds with the virtual ip.
		//
		// In tproxy tests, Envoy is not configured with a unique listener for each
		// upstream. This means the usual approach for non-tproxy tests doesn't
		// work - where we send the request to a host address mapped in to Envoy's
		// upstream listener. Instead, we exec into the container and run curl.
		//
		// We must make this request with a non-envoy user. The envoy and consul
		// users are excluded from traffic redirection rules, so instead we
		// make the request as root.
		out, err := clientService.Exec(
			context.Background(),
			[]string{"sudo", "sh", "-c", fmt.Sprintf(`
			set -e
			VIRTUAL=$(dig @localhost +short %[1]s)
			echo "Virtual IP: $VIRTUAL"
			curl -s "$VIRTUAL/debug?env=dump"
			`, virtualHostname),
			},
		)
		t.Logf("curl request to upstream virtual address\nerr = %v\nout = %s", err, out)
		require.NoError(r, err)
		require.Regexp(r, `Virtual IP: 240.0.0.\d+`, out)
		require.Contains(r, out, fmt.Sprintf("FORTIO_NAME=%s", serverName))
	})
}
