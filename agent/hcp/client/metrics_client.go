package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/protobuf/proto"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/hashicorp/consul/version"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-retryablehttp"
	hcpcfg "github.com/hashicorp/hcp-sdk-go/config"
)

const (
	// HTTP Client config
	defaultStreamTimeout = 15 * time.Second

	// Retry config
	defaultRetryWaitMin = 1 * time.Second
	defaultRetryWaitMax = 15 * time.Second
	defaultRetryMax     = 4
)

// MetricsClient exports Consul metrics in OTLP format to the HCP Telemetry Gateway.
type MetricsClient interface {
	ExportMetrics(ctx context.Context, protoMetrics *metricpb.ResourceMetrics) error
}

// cloudConfig represents cloud config for TLS abstracted in an interface for easy testing.
type CloudConfig interface {
	HCPConfig(opts ...hcpcfg.HCPConfigOption) (hcpcfg.HCPConfig, error)
}

// otlpClient is an implementation of MetricsClient with a retryable http client for retries and to honor throttle.
// It also holds default HTTP headers to add to export requests.
type otlpClient struct {
	client           *retryablehttp.Client
	headers          map[string]string
	endpointProvider TelemetryEndpointProvider
}

// TelemetryClientCfg is used to configure the MetricsClient.
type TelemetryClientCfg struct {
	CloudCfg         CloudConfig
	EndpointProvider TelemetryEndpointProvider
	Logger           hclog.Logger
}

type TelemetryEndpointProvider interface {
	Endpoint(ctx context.Context) (string, error)
}

// NewMetricsClient returns a configured MetricsClient.
// The current implementation uses otlpClient to provide retry functionality.
func NewMetricsClient(cfg *TelemetryClientCfg) (MetricsClient, error) {
	if cfg.CloudCfg == nil || cfg.Logger == nil {
		return nil, fmt.Errorf("failed to init telemetry client: provide valid TelemetryClientCfg")
	}

	c, err := newHTTPClient(cfg.CloudCfg, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to init telemetry client: %v", err)
	}

	headers := map[string]string{
		"X-HCP-Source-Channel": fmt.Sprintf("consul %s hcp-go-sdk/%s", version.GetHumanVersion(), version.Version),
		"Content-Type":         "application/x-protobuf",
	}

	return &otlpClient{
		client:           c,
		headers:          headers,
		endpointProvider: cfg.EndpointProvider,
	}, nil
}

// newHTTPClient configures the retryable HTTP client.
func newHTTPClient(cloudCfg CloudConfig, logger hclog.Logger) (*retryablehttp.Client, error) {
	hcpCfg, err := cloudCfg.HCPConfig()
	if err != nil {
		return nil, err
	}

	tlsTransport := cleanhttp.DefaultPooledTransport()
	tlsTransport.TLSClientConfig = hcpCfg.APITLSConfig()

	var transport http.RoundTripper = &oauth2.Transport{
		Base:   tlsTransport,
		Source: hcpCfg,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   defaultStreamTimeout,
	}

	retryClient := &retryablehttp.Client{
		HTTPClient:   client,
		Logger:       logger.Named("hcp_telemetry_client"),
		RetryWaitMin: defaultRetryWaitMin,
		RetryWaitMax: defaultRetryWaitMax,
		RetryMax:     defaultRetryMax,
		CheckRetry:   retryablehttp.DefaultRetryPolicy,
		Backoff:      retryablehttp.DefaultBackoff,
	}

	return retryClient, nil
}

// ExportMetrics is the single method exposed by MetricsClient to export OTLP metrics to the desired HCP endpoint.
func (o *otlpClient) ExportMetrics(ctx context.Context, protoMetrics *metricpb.ResourceMetrics) error {
	pbRequest := &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{protoMetrics},
	}

	body, err := proto.Marshal(pbRequest)
	if err != nil {
		return fmt.Errorf("failed to export metrics: %v", err)
	}

	endpoint, err := o.endpointProvider.Endpoint(ctx)
	if err != nil {
		return fmt.Errorf("fail to find metrics export endpoint: %w", err)
	}

	req, err := retryablehttp.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to export metrics: %v", err)
	}

	for k, v := range o.headers {
		req.Header.Set(k, v)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to export metrics: %v", err)
	}

	var respData bytes.Buffer
	if _, err := io.Copy(&respData, resp.Body); err != nil {
		return fmt.Errorf("failed to export metrics: %v", err)
	}

	if respData.Len() != 0 {
		var respProto colmetricpb.ExportMetricsServiceResponse
		if err := proto.Unmarshal(respData.Bytes(), &respProto); err != nil {
			return fmt.Errorf("failed to export metrics: %v", err)
		}

		if respProto.PartialSuccess != nil {
			msg := respProto.PartialSuccess.GetErrorMessage()
			return fmt.Errorf("failed to export metrics: partial success: %s", msg)
		}
	}

	return nil
}

func NewRefreshableTelemetryEndpointProvider(client Client) TelemetryEndpointProvider {
	p := &RefreshableTelemetryEndpointProvider{
		client: client,
	}
	ep, err := p.refresh(context.Background())
	p.endpoint = ep
	p.err = err
	return p
}

type RefreshableTelemetryEndpointProvider struct {
	client   Client
	endpoint string
	err      error
	mu       sync.Mutex
}

func (p *RefreshableTelemetryEndpointProvider) Endpoint(ctx context.Context) (string, error) {
	return p.endpoint, p.err
}

func (p *RefreshableTelemetryEndpointProvider) Refresh(ctx context.Context) error {
	endpoint, err := p.refresh(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		if p.endpoint == "" {
			p.err = err
		}
		return err
	}
	p.endpoint = endpoint
	p.err = nil
	return nil
}

func (p *RefreshableTelemetryEndpointProvider) refresh(ctx context.Context) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	telemetryCfg, err := p.client.FetchTelemetryConfig(reqCtx)
	if err != nil {
		return "", fmt.Errorf("failed to fetch telemetry config %w", err)
	}

	endpoint := telemetryCfg.Endpoint
	if override := telemetryCfg.MetricsOverride.Endpoint; override != "" {
		endpoint = override
	}

	if endpoint == "" {
		return "", fmt.Errorf("server not registed with management plane")
	}

	// The endpoint from the HCP gateway is a domain without scheme, so it must be added.
	url, err := url.Parse(fmt.Sprintf("https://%s/v1/metrics", endpoint))
	if err != nil {
		return "", fmt.Errorf("failed to parse url: %w", err)
	}

	return url.String(), nil
}

type StaticTelemetryEndpoint string

func (s StaticTelemetryEndpoint) Endpoint(_ context.Context) (string, error) {
	return string(s), nil
}
