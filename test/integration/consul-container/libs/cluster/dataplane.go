package cluster

import (
	"context"
	"fmt"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"strconv"
	"time"
)

func NewConsulDataplane(ctx context.Context, proxyID string, serverAddresses string, grpcPort int, node Agent, containerArgs ...string) (*LaunchInfo, error) {
	namePrefix := fmt.Sprintf("%s-consul-dataplane", node.GetDatacenter())
	containerName := utils.RandName(namePrefix)

	pod := node.GetPod()
	if pod == nil {
		return nil, fmt.Errorf("node Pod is required")
	}

	var (
		grpcPortStr = strconv.Itoa(grpcPort)
	)

	nodeName := node.GetAgentName()
	command := []string{
		"-addresses", serverAddresses,
		fmt.Sprintf("-grpc-port=%d", grpcPort),
		fmt.Sprintf("-proxy-service-id=%s", proxyID),
		fmt.Sprintf("-service-node-name=%s", nodeName),
		"-log-level=info",
		"-log-json=false",
		"-envoy-concurrency=2",
		"-tls-disabled",
		"-consul-dns-bind-port=8600",
	}

	command = append(command, containerArgs...)

	req := testcontainers.ContainerRequest{
		Image:      "consul-dataplane/release-default:1.3.0-dev",
		WaitingFor: wait.ForLog("").WithStartupTimeout(60 * time.Second),
		AutoRemove: false,
		Name:       containerName,
		Cmd:        command,
		Env:        map[string]string{},
	}

	return LaunchContainerOnNode(ctx, node, req, []string{grpcPortStr})

}
