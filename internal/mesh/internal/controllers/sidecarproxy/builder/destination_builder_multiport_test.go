// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package builder

import (
	"fmt"
	pbcatalog "github.com/hashicorp/consul/proto-public/pbcatalog/v1alpha1"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/internal/catalog"
	"github.com/hashicorp/consul/internal/mesh/internal/types/intermediate"
	"github.com/hashicorp/consul/internal/resource/resourcetest"
	pbmesh "github.com/hashicorp/consul/proto-public/pbmesh/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

func TestBuildMultiportImplicitDestinations(t *testing.T) {
	const (
		apiApp      = "api-app"
		trustDomain = "foo.consul"
		datacenter  = "dc1"
	)
	proxyCfg := &pbmesh.ProxyConfiguration{
		DynamicConfig: &pbmesh.DynamicConfig{
			Mode: pbmesh.ProxyMode_PROXY_MODE_TRANSPARENT,
			TransparentProxy: &pbmesh.TransparentProxy{
				OutboundListenerPort: 15001,
			},
		},
	}

	multiportEndpointsData := &pbcatalog.ServiceEndpoints{
		Endpoints: []*pbcatalog.Endpoint{
			{
				Addresses: []*pbcatalog.WorkloadAddress{
					{Host: "10.0.0.1"},
				},
				Ports: map[string]*pbcatalog.WorkloadPort{
					"admin-port": {Port: 8080, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
					"api-port":   {Port: 9090, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
					"mesh":       {Port: 20000, Protocol: pbcatalog.Protocol_PROTOCOL_MESH},
				},
			},
		},
	}
	apiAppEndpoints := resourcetest.Resource(catalog.ServiceEndpointsType, apiApp).
		WithOwner(resourcetest.Resource(catalog.ServiceType, apiApp).ID()).
		WithData(t, multiportEndpointsData).Build()

	apiAppIdentity := &pbresource.Reference{
		Name:    fmt.Sprintf("%s-identity", apiApp),
		Tenancy: apiAppEndpoints.Id.Tenancy,
	}

	destination1 := &intermediate.Destination{
		ServiceEndpoints: &intermediate.ServiceEndpoints{
			Resource:  apiAppEndpoints,
			Endpoints: multiportEndpointsData,
		},
		Identities: []*pbresource.Reference{apiAppIdentity},
		VirtualIPs: []string{"1.1.1.1"},
	}

	cases := map[string]struct {
		getDestinations func() []*intermediate.Destination
	}{
		// Most basic test that multiport configuration works
		"destination/multiport-l4-single-implicit-destination-tproxy": {
			getDestinations: func() []*intermediate.Destination { return []*intermediate.Destination{destination1} },
		},
		// Test shows that with multiple workloads for a service exposing the same ports, the routers
		// and clusters do not get duplicated.
		"destination/multiport-l4-single-implicit-destination-with-multiple-workloads-tproxy": {
			getDestinations: func() []*intermediate.Destination {
				mwEndpointsData := &pbcatalog.ServiceEndpoints{
					Endpoints: []*pbcatalog.Endpoint{
						{
							Addresses: []*pbcatalog.WorkloadAddress{
								{Host: "10.0.0.1"},
							},
							Ports: map[string]*pbcatalog.WorkloadPort{
								"admin-port": {Port: 8080, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
								"api-port":   {Port: 9090, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
								"mesh":       {Port: 20000, Protocol: pbcatalog.Protocol_PROTOCOL_MESH},
							},
						},
						{
							Addresses: []*pbcatalog.WorkloadAddress{
								{Host: "10.0.0.2"},
							},
							Ports: map[string]*pbcatalog.WorkloadPort{
								"admin-port": {Port: 8080, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
								"api-port":   {Port: 9090, Protocol: pbcatalog.Protocol_PROTOCOL_TCP},
								"mesh":       {Port: 20000, Protocol: pbcatalog.Protocol_PROTOCOL_MESH},
							},
						},
					},
				}
				mwEndpoints := resourcetest.Resource(catalog.ServiceEndpointsType, apiApp).
					WithOwner(resourcetest.Resource(catalog.ServiceType, apiApp).ID()).
					WithData(t, mwEndpointsData).Build()

				mwIdentity := &pbresource.Reference{
					Name:    fmt.Sprintf("%s-identity", apiApp),
					Tenancy: mwEndpoints.Id.Tenancy,
				}

				mwDestination := &intermediate.Destination{
					ServiceEndpoints: &intermediate.ServiceEndpoints{
						Resource:  mwEndpoints,
						Endpoints: mwEndpointsData,
					},
					Identities: []*pbresource.Reference{mwIdentity},
					VirtualIPs: []string{"1.1.1.1"},
				}
				return []*intermediate.Destination{mwDestination}
			},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			proxyTmpl := New(testProxyStateTemplateID(), testIdentityRef(), trustDomain, datacenter, proxyCfg).
				BuildDestinations(c.getDestinations()).
				Build()

			//sort routers because JSON does not guarantee ordering and it causes flakes
			actualRouters := proxyTmpl.ProxyState.Listeners[0].Routers
			sort.Slice(actualRouters, func(i, j int) bool {
				return actualRouters[i].String() < actualRouters[j].String()
			})

			actual := protoToJSON(t, proxyTmpl)
			expected := JSONToProxyTemplate(t, goldenValueBytes(t, name, actual, *update))

			//sort routers on listener from golden file
			expectedRouters := expected.ProxyState.Listeners[0].Routers
			sort.Slice(expectedRouters, func(i, j int) bool {
				return expectedRouters[i].String() < expectedRouters[j].String()
			})
			require.Equal(t, protoToJSON(t, expected), protoToJSON(t, proxyTmpl))
		})
	}
}
