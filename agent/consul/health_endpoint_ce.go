//go:build !consulent

package consul

import (
	"errors"

	"github.com/hashicorp/consul/agent/structs"
)

// ServiceNodes returns all the nodes registered as part of a service including health info
func (h *Health) SamenessGroupServiceNodes(args *structs.ServiceSpecificRequest, reply *structs.IndexedCheckServiceNodes) error {
	return errors.New("not supported in consul CE")
}
