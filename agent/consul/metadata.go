package consul

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/hashicorp/consul/internal/resource"
	"github.com/hashicorp/consul/internal/server"
	"github.com/hashicorp/consul/lib"
	"github.com/hashicorp/consul/proto-public/pbresource"
	pbserver "github.com/hashicorp/consul/proto-public/pbserver/v1alpha1"
)

func (s *Server) broadcastMetadata(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		if err := s.writeMetadata(ctx); err != nil {
			s.logger.Error("failed to broadcast server metadata", "error", err)
		}

		timer.Reset(lib.RandomStaggerWithRange(30*time.Second, 1*time.Minute))

		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) writeMetadata(ctx context.Context) error {
	types := s.typeRegistry.Types()
	features := make([]string, len(types))
	for idx, t := range types {
		features[idx] = fmt.Sprintf("type:%s", resource.ToGVK(t))
	}

	data, err := anypb.New(&pbserver.ServerMetadata{
		Version:       s.config.VersionWithMetadata,
		LastHeartbeat: timestamppb.Now(),
		Features:      features,
	})
	if err != nil {
		return err
	}

	_, err = s.internalResourceServiceClient.Write(ctx, &pbresource.WriteRequest{
		Resource: &pbresource.Resource{
			Id: &pbresource.ID{
				Type: server.MetadataType,
				// TODO:(boxofrad): make this a global resource once it's supported.
				Tenancy: &pbresource.Tenancy{
					Partition: "default",
					PeerName:  "local",
					Namespace: "default",
				},
				Name: string(s.config.NodeID),
			},
			Data: data,
		},
	})
	if err != nil {
		return err
	}

	return nil
}
