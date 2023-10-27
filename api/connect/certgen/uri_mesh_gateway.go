package certgen

import (
	"fmt"
	"net/url"
)

type SpiffeIDMeshGateway struct {
	Host       string
	Partition  string
	Datacenter string
}

func (id SpiffeIDMeshGateway) MatchesPartition(partition string) bool {
	return id.Partition == partition
}

func (id SpiffeIDMeshGateway) PartitionOrDefault() string {
	return id.Partition
}

// URI returns the *url.URL for this SPIFFE ID.
func (id SpiffeIDMeshGateway) URI() *url.URL {
	var result url.URL
	result.Scheme = "spiffe"
	result.Host = id.Host
	result.Path = id.uriPath()
	return &result
}

func (id SpiffeIDMeshGateway) uriPath() string {
	return fmt.Sprintf("/gateway/mesh/dc/%s", id.Datacenter)
}
