package server

import (
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/consul/internal/resource"
	"github.com/hashicorp/consul/proto-public/pbresource"
	pb "github.com/hashicorp/consul/proto-public/pbserver/v1alpha1"
)

var MetadataType = &pbresource.Type{
	Group:        "server",
	GroupVersion: "v1alpha1",
	Kind:         "ServerMetadata",
}

func RegisterTypes(r resource.Registry) {
	r.Register(resource.Registration{
		Type:     MetadataType,
		Proto:    &pb.ServerMetadata{},
		Validate: validateMetadata,
	})
}

func validateMetadata(r *pbresource.Resource) error {
	var meta pb.ServerMetadata
	if err := r.Data.UnmarshalTo(&meta); err != nil {
		return resource.NewErrDataParse(&meta, err)
	}

	var err error
	if meta.Version == "" {
		err = multierror.Append(err, resource.ErrInvalidField{
			Name:    "version",
			Wrapped: resource.ErrEmpty,
		})
	}
	if meta.LastHeartbeat == nil {
		err = multierror.Append(err, resource.ErrInvalidField{
			Name:    "last_heartbeat",
			Wrapped: resource.ErrEmpty,
		})
	}
	return err
}
