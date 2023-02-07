package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/consul/agent/consul"
	"github.com/hashicorp/consul/version"
	"github.com/hashicorp/go-checkpoint"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-uuid"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"
)

// Bump this version if the json schema for telemetry changes
// Adding new fields should not need a version bump, but removing/renaming fields should cause a version bump
const SchemaVersion = 1.0

type ConsulTelemetry struct {
	Version            string
	ACLEnabled         bool
	Server             bool
	NumClients         uint64
	NumWANAgents       uint64
	NumDCs             uint64
	UIEnabled          bool
	ServiceMeshEnabled bool
	ServiceMeshUsed    bool
	ServiceMeshProxies uint64
}

func (a *Agent) TelemetryParams() *checkpoint.ReportParams {
	ret := &checkpoint.ReportParams{}
	ret.Product = "Consul"
	sig, err := generateRandomStringURLSafe(10)
	if err != nil {
		a.logger.Info("random bytes for signature", sig)
		ret.Signature = sig
	}
	agentStats := a.delegate.Stats()
	consulStats := agentStats["consul"]

	server := consulStats["server"]
	isServer := server == "true"
	serfLANStats := agentStats["serf_lan"]

	var numClients uint64
	if serfLANStats != nil {
		numClients, err = strconv.ParseUint(serfLANStats["members"], 10, 64)
		if err != nil {
			numClients = 0
		}
	}

	serfWANStats := agentStats["serf_wan"]
	var numWANMembers uint64
	if serfWANStats != nil {
		numWANMembers, err = strconv.ParseUint(serfLANStats["members"], 10, 64)
		if err != nil {
			numWANMembers = 0
		}
	}

	var knownDCs uint64
	knownDCs, err = strconv.ParseUint(consulStats["known_datacenters"], 10, 64)
	if err != nil {
		knownDCs = 0
	}

	telemetry := ConsulTelemetry{
		Version:            version.Version,
		Server:             isServer,
		ACLEnabled:         a.config.ACLsEnabled,
		NumClients:         numClients,
		NumWANAgents:       numWANMembers,
		NumDCs:             knownDCs,
		UIEnabled:          a.config.UIConfig.Enabled,
		ServiceMeshEnabled: a.config.ConnectEnabled,
	}
	if isServer {
		svr := a.delegate.(*consul.Server)
		_, usage, err := svr.FSM().State().ServiceUsage(nil)
		if err == nil {
			serviceMeshUsage := usage.ConnectServiceInstances
			telemetry.ServiceMeshUsed = len(serviceMeshUsage) > 0
			for _, count := range serviceMeshUsage {
				telemetry.ServiceMeshProxies += uint64(count)
			}
		} else {
			a.logger.Warn("This is a server, error getting usage stats", "error", err)
		}

	}
	ret.Payload = telemetry
	ret.StartTime = time.Now()
	a.logger.Info("Full Telemetry Object", "payload", telemetry)
	return ret
}

func generateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return nil, err
	}

	return b, nil
}

func generateRandomStringURLSafe(n int) (string, error) {
	b, err := generateRandomBytes(n)
	return base64.URLEncoding.EncodeToString(b), err
}

// Report sends telemetry information to checkpoint
func (a *Agent) ReportTelemetry(ctx context.Context, r *checkpoint.ReportParams) error {
	if disabled := os.Getenv("CHECKPOINT_DISABLE"); disabled != "" {
		return nil
	}

	req, err := productTelemetryRequest(r)
	if err != nil {
		return err
	}

	client := cleanhttp.DefaultClient()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return fmt.Errorf("Unknown status: %d", resp.StatusCode)
	}

	return nil
}

// productTelemetryRequest creates a request object for making a report
func productTelemetryRequest(r *checkpoint.ReportParams) (*http.Request, error) {
	// Populate some fields automatically if we can
	if r.RunID == "" {
		uuid, err := uuid.GenerateUUID()
		if err != nil {
			return nil, err
		}
		r.RunID = uuid
	}
	if r.Arch == "" {
		r.Arch = runtime.GOARCH
	}
	if r.OS == "" {
		r.OS = runtime.GOOS
	}
	if r.Signature == "" {
		r.Signature = "todo-random-bytes"
	}
	r.Product = "Consul"
	r.SchemaVersion = "1.0"

	b, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}

	u := &url.URL{
		Scheme: "http",
		Host:   "127.0.0.1:8000", //TODO change this to a new ngrok URL when testing with k8s clusters
		Path:   fmt.Sprintf("/v1/telemetry/%s", r.Product),
	}

	req, err := http.NewRequest("POST", u.String(), bytes.NewReader(b))
	fmt.Printf("http req to checkpoint %s\n", req.URL.String())
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HashiCorp/go-checkpoint")

	return req, nil
}
