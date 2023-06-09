// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/internal/resource"
	"github.com/hashicorp/consul/internal/storage"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

func (s *Server) List(ctx context.Context, req *pbresource.ListRequest) (*pbresource.ListResponse, error) {
	if err := validateListRequest(req); err != nil {
		return nil, err
	}

	// check type
	reg, err := s.resolveType(req.Type)
	if err != nil {
		return nil, err
	}

	authz, err := s.getAuthorizer(tokenFromContext(ctx))
	if err != nil {
		return nil, err
	}

	// check acls
	err = reg.ACLs.List(authz, req.Tenancy)
	switch {
	case acl.IsErrPermissionDenied(err):
		return nil, status.Error(codes.PermissionDenied, err.Error())
	case err != nil:
		return nil, status.Errorf(codes.Internal, "failed list acl: %v", err)
	}

	resources, err := s.Backend.List(
		ctx,
		readConsistencyFrom(ctx),
		storage.UnversionedTypeFrom(req.Type),
		req.Tenancy,
		req.NamePrefix,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed list: %v", err)
	}

	result := make([]*pbresource.Resource, 0)
	for _, r := range resources {
		if !resource.EqualType(req.Type, r.Id.Type) {
			r, err = s.Translate(r, req.Type)
			switch {
			case errors.Is(err, errCannotTranslate):
				continue
			case err != nil:
				return nil, status.Errorf(codes.Internal, "failed to translate resource: %v", err)
			}
		}

		// filter out items that don't pass read ACLs
		err = reg.ACLs.Read(authz, r.Id)
		switch {
		case acl.IsErrPermissionDenied(err):
			continue
		case err != nil:
			return nil, status.Errorf(codes.Internal, "failed read acl: %v", err)
		}
		result = append(result, r)
	}
	return &pbresource.ListResponse{Resources: result}, nil
}

func validateListRequest(req *pbresource.ListRequest) error {
	var field string
	switch {
	case req.Type == nil:
		field = "type"
	case req.Tenancy == nil:
		field = "tenancy"
	default:
		return nil
	}
	return status.Errorf(codes.InvalidArgument, "%s is required", field)
}
