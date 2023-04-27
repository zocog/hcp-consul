package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	colpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"

	"github.com/hashicorp/consul/version"
	"github.com/hashicorp/go-hclog"
)

func TestNewMetricsClient(t *testing.T) {
	for name, test := range map[string]struct {
		wantErr string
		cfg     *TelemetryClientCfg
	}{
		"success": {
			cfg: &TelemetryClientCfg{
				Logger:   hclog.New(&hclog.LoggerOptions{Output: io.Discard}),
				CloudCfg: &MockCloudCfg{},
			},
		},
		"failsWithoutCloudCfg": {
			wantErr: "failed to init telemetry client",
			cfg: &TelemetryClientCfg{
				Logger:   hclog.New(&hclog.LoggerOptions{Output: io.Discard}),
				CloudCfg: nil,
			},
		},
		"failsWithoutLogger": {
			wantErr: "failed to init telemetry client",
			cfg: &TelemetryClientCfg{
				Logger:   nil,
				CloudCfg: &MockErrCloudCfg{},
			},
		},
		"failsHCPConfig": {
			wantErr: "failed to init telemetry client",
			cfg: &TelemetryClientCfg{
				Logger:   hclog.New(&hclog.LoggerOptions{Output: io.Discard}),
				CloudCfg: &MockErrCloudCfg{},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewMetricsClient(test.cfg)
			if test.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.wantErr)
				return
			}

			require.Nil(t, err)
			require.NotNil(t, client)
		})
	}
}

func TestExportMetrics(t *testing.T) {
	for name, test := range map[string]struct {
		wantErr string
		status  int
	}{
		"success": {
			status: http.StatusOK,
		},
		"failsWithNonRetryableError": {
			status:  http.StatusBadRequest,
			wantErr: "failed to export metrics",
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, r.Header.Get("Content-Type"), "application/x-protobuf")

				require.Equal(t, r.Header.Get("Authorization"), "Bearer test-token")
				require.Equal(t, r.Header.Get("X-HCP-Source-Channel"), fmt.Sprintf("consul %s hcp-go-sdk/%s", version.GetHumanVersion(), version.Version))

				body := colpb.ExportMetricsServiceResponse{}

				if test.wantErr != "" {
					body.PartialSuccess = &colpb.ExportMetricsPartialSuccess{
						ErrorMessage: "partial failure",
					}
				}
				bytes, err := proto.Marshal(&body)

				require.NoError(t, err)

				w.Header().Set("Content-Type", "application/x-protobuf")
				w.WriteHeader(test.status)
				w.Write(bytes)
			}))
			defer srv.Close()

			cfg := &TelemetryClientCfg{
				Logger:           hclog.New(&hclog.LoggerOptions{Output: io.Discard}),
				CloudCfg:         MockCloudCfg{},
				EndpointProvider: StaticTelemetryEndpoint(srv.URL),
			}

			client, err := NewMetricsClient(cfg)
			require.NoError(t, err)

			ctx := context.Background()
			metrics := &metricpb.ResourceMetrics{}
			err = client.ExportMetrics(ctx, metrics)

			if test.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), test.wantErr)
				return
			}

			require.NoError(t, err)
		})
	}

}
