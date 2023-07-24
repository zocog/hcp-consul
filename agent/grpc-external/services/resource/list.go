// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resource

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/internal/storage"
	"github.com/hashicorp/consul/proto-public/pbresource"
	external "github.com/hashicorp/consul/agent/grpc-external"
	"google.golang.org/grpc"
)

func (s *Server) List(ctx context.Context, req *pbresource.ListRequest) (*pbresource.ListResponse, error) {
	if err := validateListRequest(req); err != nil {
		return nil, err
	}

	rpcInfo, err := readRequestFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var resp *pbresource.ListResponse
	handled, err := s.ForwardRPC(rpcInfo, func(cc *grpc.ClientConn) error {
		ctx := external.ForwardMetadataContext(ctx)
		var err error
		resp, err = pbresource.NewResourceServiceClient(cc).List(ctx, req)
		return err
	})
	if handled || err != nil {
		return resp, err
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
	for _, resource := range resources {
		// filter out non-matching GroupVersion
		if resource.Id.Type.GroupVersion != req.Type.GroupVersion {
			continue
		}

		// filter out items that don't pass read ACLs
		err = reg.ACLs.Read(authz, resource.Id)
		switch {
		case acl.IsErrPermissionDenied(err):
			continue
		case err != nil:
			return nil, status.Errorf(codes.Internal, "failed read acl: %v", err)
		}
		result = append(result, resource)
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
