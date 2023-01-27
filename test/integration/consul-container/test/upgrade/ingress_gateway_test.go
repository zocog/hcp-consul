package upgrade

import (
	"testing"

	libassert "github.com/hashicorp/consul/test/integration/consul-container/libs/assert"
)

// These tests adapt BATS-based tests from test/integration/connect/case-ingress-gateway*

func TestIngressGatewayGRPC(t *testing.T) {
	t.Skip("TODO")
	// setup
	// upsert config entry making grpc default protocol for s1
	// upsert config entry for ingress-gateway ig1, protocol grpc, service s1
	// create service for s1 with connect proxy protocol grpc
	// create `ingress-gateway` service `ingress-gateway`

	// checks
	// ingress-gateway proxy admin up
	// s1 proxy admin up
	// s2 proxy admin up (??? we don't care about s2 ???)
	// s1 proxy listener has right cert
	// ig1 has healthy endpoint for s1

	// tests
	// fortio grpcping ig1 listener
	libassert.GRPCPing(t, "<addr>")
}

func TestIngressGatewayHTTP(t *testing.T) {
	t.Skip("TODO")
	// setup
	// upsert config entry making grpc default protocol for global
	// TODO ^ not sure why this is global and s1 in GRPC
	// upsert config entry for ingress-gateway ig1, protocol grpc, service s1
	// - listener points at service `router`
	// 	- add request headers: 1 new, 1 existing
	// 	- set request headers: 1 existing, 1 new, to client IP
	//  - add response headers: 1 new, 1 existing
	//  - set response headers: 1 existing
	//  - remove response header: 1 existing
	// upsert config entry for `service-router` `router`:
	// - prefix matching `/s1` goes to service s1
	// - prefix matching `/s2` goes to service s2
	// create `ingress-gateway` service `ingress-gateway`

	// checks
	// ingress-gateway proxy admin up
	// s1 proxy admin up
	// s2 proxy admin up
	// s1 proxy listener has right cert
	// s2 proxy listener has right cert
	// ig1 has healthy endpoints for s1
	// ig1 has healthy endpoints for s2
	// TODO ^ ??? s1 and s2 aren't direct listeners, only in `router`, so why are they endpoints?

	// tests
	// fortio name should be sX for /sX prefix on router
	libassert.FortioName("s1", "router.ingress.consul:9999", "/s1")
	libassert.FortioName("s2", "router.ingress.consul:9999", "/s2")
	// HTTP resquest header manipulation
	// TODO: HTTP GET on router.ingress.consul/s2/debug?env=dump, and verify adjusted headers
	// HTTP response header manipulation
	// TODO: HTTP GET on router.ingress.consul/s2/echo?<headers>, and verify adjusted headers
}

func TestIngressGatewayMultipleServices(t *testing.T) {
	t.Skip("TODO")
	// TODO: I might more accurately call this "ServicesWildcard"

	// setup
	// upsert config entry making http default protocol globally
	// upsert config entry for ingress-gateway:
	//   - port 9999: services: *, set Defaults
	//   - port 9998: services: s1 for host test.example.com, set Defaults

	// create `ingress-gateway` service `ingress-gateway`

	// checks
	// ingress-gateway proxy admin up
	// s1 proxy admin up
	// s2 proxy admin up
	// s1 proxy listener has right cert
	// s2 proxy listener has right cert
	// s1 proxy has healthy instances
	// s2 proxy has healthy instances
	// ingress-gateway has healthy endpoints for s1
	// ingress-gateway has healthy endpoints for s2

	// tests
	// TODO: maybe these should be moved to basic HTTP tests?
	// s2 check envoy config .circuit_breakers.thresholds[0] for parameters we set in Defaults
	// s2 check envoy config .outlier_detection for our Default
	// TODO: we don't check these ^ on s1?
	// connect to s1 using host name
	libassert.FortioName(t, "s1", "s1.ingress.consul:9999")
	// connect to s2 using host name
	libassert.FortioName(t, "s2", "s2.ingress.consul:9999")
	// connect to s1 using user-specified host name, on our other listener
	libassert.FortioName(t, "s2", "test.example.com:9998")
}

func TestIngressGatewayPeeringFailover(t *testing.T) {
	t.Skip("TODO: multi cluster")
}

func TestIngressGatewaySDS(t *testing.T) {
	t.Skip("TODO")
	// setup
	// upsert config for global proxy-defaults, protocol http
	// upsert config for ingress-gateway named ingress-gateway:
	// 	- port 9999 listener for services *, with tls from sds wildcard.ingress.consul
	// 	- port 9998 listener for:
	//		- service s1, with tls for host foo.example.com from sds
	//		- service s2, with tls for host www.example.com from sds
	// create ingress-gateway service with proxy.config.envoy_extra_static_clusters_json configured to 127.0.0.1:1234

	// checks
	// ingress-gateway proxy admin up
	// s1 proxy admin up
	// s2 proxy admin up
	// s1 proxy listener has right cert
	// s2 proxy listener has right cert
	// ingress-gateway has healthy endpoints for s1
	// ingress-gateway has healthy endpoints for s2

	// tests
	// automatic DNS for service name
	libassert.FortioName(t, "s1", "https://s1.ingress.consul:9999")
	// automatic DNS for service name
	libassert.FortioName(t, "s2", "https://s2.ingress.consul:9999")
	// manual host name
	libassert.FortioName(t, "s1", "https://foo.example.com:9998")
	// TODO: ^ not really sure how we're able to do GRPC when the services
	// have a protocol of HTTP declared
	// ingress should serve SDS-supplied cert for wildcard
	// assert_cert_signed_by_ca ca-root.crt localhost:9999 *.ingress.consul
	// ingress should serve SDS-supplied cert for specific service
	// assert_cert_signed_by_ca ca-root.crt localhost:9998 foo.example.com
	// ingress should serve SDS-supplied cert for second specific service
	// assert_cert_signed_by_ca ca-root.crt localhost:9998 www.example.com
	// TODO: since this ^ func is only used by this test suite, we don't need it in libassert
}

func TestIngressGatewaySimple(t *testing.T) {
	t.Skip("TODO")
	// setup
	// upsert config entry for ingress-gateway:
	//   - port 9999: services: s1 set Defaults

	// create `ingress-gateway` service `ingress-gateway`

	// checks
	// ingress-gateway proxy admin up
	// s1 proxy admin up
	// s2 proxy admin up
	// TODO ^why do we care about s2?
	// s1 proxy listener has right cert
	// ingress-gateway has healthy endpoints for s1

	// tests
	// TODO: maybe these should be moved to basic HTTP tests?
	// s1 check envoy config .circuit_breakers.thresholds[0] for parameters we set in Defaults
	// connect to s1 using host name
	libassert.HTTPEcho(t, "http://localhost:9999")
}

func TestIngressGatewayTLS(t *testing.T) {
	t.Skip("TODO")
	// setup
	// upsert config entry making http default protocol globally
	// upsert config entry for ingress-gateway:
	//   - port 9998: services: s1
	//   - port 9999: services: s1 for host test.example.com

	// create `ingress-gateway` service `ingress-gateway`

	// checks
	// ingress-gateway proxy admin up
	// s1 proxy admin up
	// s2 proxy admin up
	// TODO: ^ not sure why we check this
	// s1 proxy listener has right cert
	// s1 proxy has healthy instances
	// ingress-gateway has healthy endpoints for s1

	// tests
	// TODO: verify.bats does assert_dnssan_in_cert for *.ingress.consul,
	// I would expect curl to have that covered
	libassert.HTTPEcho("https://s1.ingress.consul:9998")
	libassert.HTTPEcho("https://test.example.com:9999")
}

func TestIngressGatewayMeshGatewayResolver(t *testing.T) {
	t.Skip("TODO: multi-cluster")
}
