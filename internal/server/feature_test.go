package server_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	svctest "github.com/hashicorp/consul/agent/grpc-external/services/resource/testing"
	"github.com/hashicorp/consul/internal/server"
	"github.com/hashicorp/consul/proto-public/pbresource"
	pb "github.com/hashicorp/consul/proto-public/pbserver/v1alpha1"
	"github.com/hashicorp/consul/sdk/testutil"
)

func TestFlagWatcher(t *testing.T) {
	client := svctest.RunResourceService(t, server.RegisterTypes)
	ctx := testutil.TestContext(t)

	tenancy := &pbresource.Tenancy{
		Partition: "default",
		PeerName:  "local",
		Namespace: "default",
	}

	watcher := server.NewFlagWatcher(client)
	go watcher.Run(ctx)

	require.False(t, watcher.Supported("flag-a"))

	_, err := client.Write(ctx, &pbresource.WriteRequest{
		Resource: &pbresource.Resource{
			Id: &pbresource.ID{
				Type:    server.MetadataType,
				Tenancy: tenancy,
				Name:    "server-a",
			},
			Data: any(t, &pb.ServerMetadata{
				Version:       "1.2.3",
				LastHeartbeat: timestamppb.Now(),
				Features:      []string{"flag-a"},
			}),
		},
	})
	require.NoError(t, err)

	<-watcher.Change()
	require.True(t, watcher.Supported("flag-a"))

	_, err = client.Write(ctx, &pbresource.WriteRequest{
		Resource: &pbresource.Resource{
			Id: &pbresource.ID{
				Type:    server.MetadataType,
				Tenancy: tenancy,
				Name:    "server-b",
			},
			Data: any(t, &pb.ServerMetadata{
				Version:       "1.2.3",
				LastHeartbeat: timestamppb.Now(),
				Features:      []string{},
			}),
		},
	})
	require.NoError(t, err)

	<-watcher.Change()
	require.False(t, watcher.Supported("flag-a"))
}

func any(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()

	any, err := anypb.New(msg)
	require.NoError(t, err)

	return any
}
