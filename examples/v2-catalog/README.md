## Testing steps

These steps outline how to configure and validate setting up a service named `web`
with an explicit upstream of a single port on the `api` service utilizing the
catalog v2 model with consul dataplane.

### Prerequisites

* Consul and consul-dataplane binaries built locally from the appropriate branches
* [`fake-service`](https://github.com/nicholasjackson/fake-service/) binary

### Start Consul dev agent

```bash
./bin/consul agent -dev -log-level=trace -hcl 'experiments=["resource-apis"]'
```

### Register resources

```bash
for f in `ls ../consul-catalog-v2-examples/explicit-single-port/ | grep -iv readme `; do echo $f; consul resource apply -f ../consul-catalog-v2-examples/explicit-single-port/$f; done
```

### Start our services

In separate terminal tabs, start the `api` and `web` services that we will use:

```bash
LISTEN_ADDR=0.0.0.0:19091 NAME=api fake-service
```

```bash
LISTEN_ADDR=0.0.0.0:19090 NAME=web UPSTREAM_URIS=http://localhost:1234 fake-service
```

### Start consul-dataplane for each service

In separate terminal tabs, start consul-dataplane processes for each `web` and `api` workloads:

api:

```bash
consul-dataplane -addresses=127.0.0.1 -tls-disabled -proxy-id=api-123abc -proxy-namespace=default -proxy-partition=default
```

web:

```bash
consul-dataplane -addresses=127.0.0.1 -tls-disabled -proxy-id=web-123abc -proxy-namespace=default -proxy-partition=default -envoy-admin-bind-port=19001 -graceful-port=20301
```

### Check that services can connect

To check that `web` can talk to `api` go to http://127.0.0.1:19090 in your browser.
