// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package builder

import (
	"flag"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	update = flag.Bool("update", false, "update the golden files of this test")
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func protoToJSON(t *testing.T, pb proto.Message) string {
	t.Helper()
	m := protojson.MarshalOptions{
		Indent: "  ",
	}
	gotJSON, err := m.Marshal(pb)
	require.NoError(t, err)
	return string(gotJSON)
}

func JSONToProxyTemplate(t *testing.T, json []byte) *pbmesh.ProxyStateTemplate {
	t.Helper()
	proxyTemplate := &pbmesh.ProxyStateTemplate{}
	m := protojson.UnmarshalOptions{}
	err := m.Unmarshal(json, proxyTemplate)
	require.NoError(t, err)
	return proxyTemplate
}

func goldenValue(t *testing.T, goldenFile string, actual string, update bool) string {
	t.Helper()
	return string(goldenValueBytes(t, goldenFile, actual, update))
}

func goldenValueBytes(t *testing.T, goldenFile string, actual string, update bool) []byte {
	t.Helper()
	goldenPath := filepath.Join("testdata", goldenFile) + ".golden"

	if update {
		bytes := []byte(actual)
		err := os.WriteFile(goldenPath, bytes, 0644)
		require.NoError(t, err)

		return bytes
	}

	content, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	return content
}
