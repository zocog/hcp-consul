#!/usr/bin/env bats

load helpers

@test "s1 proxy is running correct version" {
  assert_envoy_version_2 19000 envoy_s1-sidecar-proxy_1
}

@test "s1 proxy admin is up on :19000" {
  retry_default curl -f -s envoy_s1-sidecar-proxy_1:19000/stats -o /dev/null
}

@test "s2 proxy admin is up on :19001" {
  retry_default curl -f -s envoy_s2-sidecar-proxy_1:19001/stats -o /dev/null
}

@test "s1 proxy listener should be up and have right cert" {
  assert_proxy_presents_cert_uri envoy_s1-sidecar-proxy_1:21000 s1
}

@test "s2 proxy listener should be up and have right cert" {
  assert_proxy_presents_cert_uri envoy_s2-sidecar-proxy_1:21001 s2
}

@test "s2 proxy should be healthy" {
  assert_service_has_healthy_instances s2 1
}
#
@test "s1 upstream should have healthy endpoints for s2" {
  assert_upstream_has_endpoints_in_status envoy_s1-sidecar-proxy_1:19000 s2.default.primary HEALTHY 1
}

@test "s1 upstream should be able to connect to s2" {
  run retry_default curl -s -f -d hello envoy_s1-sidecar-proxy_1:5000
  [ "$status" -eq 0 ]
  [ "$output" = "hello" ]
}

@test "s1 proxy should have been configured with one rbac listener filter at L4" {
  LISTEN_FILTERS=$(get_envoy_listener_filters envoy_s1-sidecar-proxy_1:19000)
  PUB=$(echo "$LISTEN_FILTERS" | grep -E "^public_listener:" | cut -f 2 -d ' ' )
  UPS=$(echo "$LISTEN_FILTERS" | grep -E "^(default\/default\/)?s2:" | cut -f 2 -d ' ' )

  echo "LISTEN_FILTERS = $LISTEN_FILTERS"
  echo "PUB = $PUB"
  echo "UPS = $UPS"

  [ "$PUB" = "envoy.filters.network.rbac,envoy.filters.network.tcp_proxy" ]
  [ "$UPS" = "envoy.filters.network.tcp_proxy" ]
}
