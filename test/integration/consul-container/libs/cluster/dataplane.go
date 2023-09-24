package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type ConsulDataplaneContainer struct {
	ctx               context.Context
	container         testcontainers.Container
	ip                string
	appPort           []int
	serviceName       string
	externalAdminPort int
	internalAdminPort int
}

func (g ConsulDataplaneContainer) Restart() error {
	var deferClean utils.ResettableDefer
	defer deferClean.Execute()

	if utils.FollowLog {
		if err := g.container.StopLogProducer(); err != nil {
			return fmt.Errorf("stopping log producer: %w", err)
		}
	}

	fmt.Printf("Stopping container: %s\n", g.GetName())
	err := g.container.Stop(g.ctx, nil)
	if err != nil {
		return fmt.Errorf("error stopping sidecar container %s", err)
	}

	fmt.Printf("Starting container: %s\n", g.GetName())
	err = g.container.Start(g.ctx)
	if err != nil {
		return fmt.Errorf("error starting sidecar container %s", err)
	}

	if utils.FollowLog {
		if err := g.container.StartLogProducer(g.ctx); err != nil {
			return fmt.Errorf("starting log producer: %w", err)
		}
		g.container.FollowOutput(&LogConsumer{})
		deferClean.Add(func() {
			_ = g.container.StopLogProducer()
		})
	}

	return nil
}

func (g ConsulDataplaneContainer) Start() error {
	if g.container == nil {
		return fmt.Errorf("container has not been initialized")
	}
	return g.container.Start(g.ctx)
}

func (g ConsulDataplaneContainer) Stop() error {
	if g.container == nil {
		return fmt.Errorf("container has not been initialized")
	}
	return g.container.Stop(context.Background(), nil)
}

func (g ConsulDataplaneContainer) Terminate() error {
	return errors.New("not implemented")
}

func (g ConsulDataplaneContainer) GetAddr() (string, int) {
	return g.ip, g.appPort[0]
}

func (g ConsulDataplaneContainer) GetAddrs() (string, []int) {
	return g.ip, g.appPort
}

func (g ConsulDataplaneContainer) GetLogs() (string, error) {
	rc, err := g.container.Logs(context.Background())
	if err != nil {
		return "", fmt.Errorf("could not get logs for connect service %s: %w", g.GetServiceName(), err)
	}
	defer rc.Close()

	out, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("could not read from logs for connect service %s: %w", g.GetServiceName(), err)
	}
	return string(out), nil
}

func (g ConsulDataplaneContainer) GetName() string {
	name, err := g.container.Name(g.ctx)
	if err != nil {
		return ""
	}
	return name
}

func (g ConsulDataplaneContainer) GetPort(port int) (int, error) {
	return 0, errors.New("not implemented")
}

func (g ConsulDataplaneContainer) GetServiceName() string {
	return g.serviceName
}

// GetAdminAddr returns the external admin port
func (g ConsulDataplaneContainer) GetAdminAddr() (string, int) {
	return "localhost", g.externalAdminPort
}

func (g ConsulDataplaneContainer) Export(partition, peer string, client *api.Client) error {
	return fmt.Errorf("ConnectContainer export unimplemented")
}

func (g ConsulDataplaneContainer) Exec(ctx context.Context, cmd []string) (string, error) {
	exitCode, reader, err := g.container.Exec(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("exec with error %s", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("exec with exit code %d", exitCode)
	}
	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("error reading from exec output: %w", err)
	}
	return string(buf), nil
}

func (g ConsulDataplaneContainer) GetStatus() (string, error) {
	state, err := g.container.State(g.ctx)
	return state.Status, err
}

func NewConsulDataplane(ctx context.Context, proxyID string, serverAddresses string, grpcPort int, serviceBindPorts []int,
	node Agent, tproxy bool, bootstrapToken string, containerArgs ...string) (*ConsulDataplaneContainer, error) {
	namePrefix := fmt.Sprintf("%s-consul-dataplane-%s", node.GetDatacenter(), proxyID)
	containerName := utils.RandName(namePrefix)

	internalAdminPort, err := node.ClaimAdminPort()
	if err != nil {
		return nil, err
	}

	pod := node.GetPod()
	if pod == nil {
		return nil, fmt.Errorf("node Pod is required")
	}

	var (
		appPortStrs  []string
		adminPortStr = strconv.Itoa(internalAdminPort)
	)

	for _, port := range serviceBindPorts {
		appPortStrs = append(appPortStrs, strconv.Itoa(port))
	}

	// expose the app ports and the envoy adminPortStr on the agent container
	exposedPorts := make([]string, len(appPortStrs))
	copy(exposedPorts, appPortStrs)
	exposedPorts = append(exposedPorts, adminPortStr)

	command := []string{
		"consul-dataplane",
		"-addresses", serverAddresses,
		fmt.Sprintf("-grpc-port=%d", grpcPort),
		fmt.Sprintf("-proxy-id=%s", proxyID),
		"-proxy-namespace=default",
		"-proxy-partition=default",
		"-log-level=trace",
		"-log-json=false",
		"-envoy-drain-time-seconds=90",
		"-tls-disabled",
		fmt.Sprintf("-envoy-admin-bind-port=%d", internalAdminPort),
	}

	if bootstrapToken != "" {
		command = append(command,
			"-credential-type=static",
			fmt.Sprintf("-static-token=%s", bootstrapToken))
	}

	command = append(command, containerArgs...)

	req := testcontainers.ContainerRequest{
		Image:      "consul-dataplane:local",
		WaitingFor: wait.ForLog("").WithStartupTimeout(60 * time.Second),
		AutoRemove: false,
		Name:       containerName,
		Cmd:        command,
		Env:        map[string]string{},
	}

	if tproxy {
		req.Entrypoint = []string{"sh", "/bin/tproxy-startup.sh"}
		req.Env["REDIRECT_TRAFFIC_ARGS"] = strings.Join(
			[]string{
				// TODO once we run this on a different pod from Consul agents, we can eliminate most of this.
				"-exclude-inbound-port", fmt.Sprint(internalAdminPort),
				"-exclude-inbound-port", "8300",
				"-exclude-inbound-port", "8301",
				"-exclude-inbound-port", "8302",
				"-exclude-inbound-port", "8500",
				"-exclude-inbound-port", "8502",
				"-exclude-inbound-port", "8600",
				"-proxy-inbound-port", "20000",
				"-consul-dns-ip", "127.0.0.1",
				"-consul-dns-port", "8600",
			},
			" ",
		)
		req.CapAdd = append(req.CapAdd, "NET_ADMIN")
	}

	info, err := LaunchContainerOnNode(ctx, node, req, exposedPorts)
	if err != nil {
		return nil, err
	}
	out := &ConsulDataplaneContainer{
		ctx:               ctx,
		container:         info.Container,
		ip:                info.IP,
		serviceName:       containerName,
		externalAdminPort: info.MappedPorts[adminPortStr].Int(),
		internalAdminPort: internalAdminPort,
	}

	for _, port := range appPortStrs {
		out.appPort = append(out.appPort, info.MappedPorts[port].Int())
	}

	fmt.Printf("NewConsulDataplane: proxyID %s, mapped App Port %d, service bind port %v\n",
		proxyID, out.appPort, serviceBindPorts)
	fmt.Printf("NewConsulDataplane: proxyID %s, , mapped admin port %d, admin port %d\n",
		proxyID, out.externalAdminPort, internalAdminPort)

	fmt.Printf("NewConsulDataplane out: %+v", out)
	fmt.Printf("NewConsulDataplane info: %+v", info)

	return out, nil
}
