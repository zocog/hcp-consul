// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package builder

import (
	"testing"

	"github.com/stretchr/testify/require"

	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v1alpha1"
)

func TestBuildLocalApp_Multiport(t *testing.T) {
	cases := map[string]struct {
		workload *pbcatalog.Workload
	}{
		"l4-multiport-single-workload-address": {
			workload: &pbcatalog.Workload{
				Addresses: []*pbcatalog.WorkloadAddress{
					{
						Host: "10.0.0.1",
					},
				},
				Ports: map[string]*pbcatalog.WorkloadPort{
					"admin-port": {Port: 8080, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
					"api-port":   {Port: 9090, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
					"mesh":       {Port: 20000, Protocol: pbcatalog.Protocol_PROTOCOL_MESH},
				},
			},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			proxyTmpl := New(testProxyStateTemplateID(), testIdentityRef(), "foo.consul", "dc1", nil).
				BuildLocalApp(c.workload).
				Build()
			actual := protoToJSON(t, proxyTmpl)
			expected := goldenValue(t, name, actual, *update)

			require.JSONEq(t, expected, actual)
		})
	}
}
