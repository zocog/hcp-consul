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

type ConsulDataplaneContainer struct {
	ctx         context.Context
	container   testcontainers.Container
	ip          string
	grpcPort    int
	serviceName string
}

func (c ConsulDataplaneContainer) Terminate() error {
	return TerminateContainer(c.ctx, c.container, true)
}

func NewConsulDataplane(ctx context.Context, proxyID string, serverAddresses string, grpcPort int, node Agent, containerArgs ...string) (*ConsulDataplaneContainer, error) {
	namePrefix := fmt.Sprintf("%s-consul-dataplane-%s", node.GetDatacenter(), proxyID)
	containerName := utils.RandName(namePrefix)

	pod := node.GetPod()
	if pod == nil {
		return nil, fmt.Errorf("node Pod is required")
	}

	var (
		grpcPortStr = strconv.Itoa(grpcPort)
	)

	command := []string{
		"-addresses", serverAddresses,
		fmt.Sprintf("-grpc-port=%d", grpcPort),
		fmt.Sprintf("-proxy-service-id=%s", proxyID),
		fmt.Sprintf("-service-node-name=%s", node.GetName()),
		"-log-level=info",
		"-log-json=false",
		"-envoy-concurrency=2",
		"-tls-disabled",
		"-consul-dns-bind-port=8601",
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

	info, err := LaunchContainerOnNode(ctx, node, req, []string{grpcPortStr})
	if err != nil {
		return nil, err
	}
	out := &ConsulDataplaneContainer{
		ctx:         ctx,
		container:   info.Container,
		ip:          info.IP,
		grpcPort:    info.MappedPorts[grpcPortStr].Int(),
		serviceName: containerName,
	}
	return out, nil

}
