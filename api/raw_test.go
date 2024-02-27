package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	pbmulticluster "github.com/hashicorp/consul/proto-public/pbmulticluster/v2"
	"github.com/hashicorp/consul/sdk/testutil"
)

func TestAPI_RawV2ExportedServices(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.EnableDebug = true
	})

	defer s.Stop()

	endpoint := strings.ToLower(fmt.Sprintf("/api/multicluster/v2/exportedservices/e1"))
	wResp := &WriteResponse{}

	var consumers []map[string]any
	consumers = append(consumers, map[string]any{"peer": "p1"})
	data := map[string]any{"consumers": consumers}
	data["services"] = []string{"s1"}
	wReq := &WriteRequest{
		Metadata: nil,
		Data:     data,
		Owner:    nil,
	}

	_, err := c.Raw().Write(endpoint, wReq, wResp, &WriteOptions{Datacenter: "dc1"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if wResp.ID.Name == "" {
		t.Fatalf("no write response")
	}

	qOpts := &QueryOptions{Datacenter: "dc1"}
	var out map[string]interface{}
	_, err = c.Raw().Query(endpoint, &out, qOpts)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	respData, err := json.Marshal(out["data"])
	readData := &pbmulticluster.ExportedServices{}
	if err = protojson.Unmarshal(respData, readData); err != nil {
		t.Fatalf("invalid read response")
	}
	if len(readData.Services) != 1 {
		t.Fatalf("incorrect resource data")
	}

	_, err = c.Raw().Delete(endpoint, qOpts)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	out = make(map[string]interface{})
	_, err = c.Raw().Query(endpoint, &out, qOpts)
	require.ErrorContains(t, err, "404")
}
