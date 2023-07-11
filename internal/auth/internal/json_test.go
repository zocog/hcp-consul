package internal

import (
	"fmt"
	"testing"

	pbauth "github.com/hashicorp/consul/proto-public/pbauth/v1alpha1"
	"github.com/hashicorp/consul/proto-public/pbresource"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestProtobufToJson(t *testing.T) {
	intentions := &pbauth.Intentions{
		Destination: &pbresource.Reference{
			Name: "api",
			Type: &pbresource.Type{Group: "auth", GroupVersion: "v1alpha1", Kind: "WorkloadIdentity"},
		},

		Action: pbauth.Action_ACTION_ALLOW,

		Rules: []*pbauth.Rule{
			{
				Sources: []*pbauth.Source{
					{
						SourceRef: &pbauth.Source_IdentityRef{
							IdentityRef: &pbauth.IdentityRef{
								Name:                "web",
								Namespace:           "web",
								PeerOrPartitionOrSg: &pbauth.IdentityRef_Peer{Peer: "web-peer"},
							},
						},
					},
				},
			},
		},
	}

	res, err := protojson.Marshal(intentions)
	require.NoError(t, err)
	fmt.Println(string(res))

	intentions = &pbauth.Intentions{
		Destination: &pbresource.Reference{
			Name: "api",
			Type: &pbresource.Type{Group: "auth", GroupVersion: "v1alpha1", Kind: "WorkloadIdentity"},
		},

		Action: pbauth.Action_ACTION_ALLOW,

		Rules: []*pbauth.Rule{
			{
				Sources: []*pbauth.Source{
					{
						SourceRef: &pbauth.Source_NamespaceRef{
							NamespaceRef: &pbauth.NamespaceRef{
								Namespace:           "web",
								PeerOrPartitionOrSg: &pbauth.NamespaceRef_Partition{Partition: "web-partition"},
							},
						},
					},
				},
			},
		},
	}

	res, err = protojson.Marshal(intentions)
	require.NoError(t, err)
	fmt.Println(string(res))

	intentions = &pbauth.Intentions{
		Destination: &pbresource.Reference{
			Name: "api",
			Type: &pbresource.Type{Group: "auth", GroupVersion: "v1alpha1", Kind: "WorkloadIdentity"},
		},

		Action: pbauth.Action_ACTION_ALLOW,

		Rules: []*pbauth.Rule{
			{
				Sources: []*pbauth.Source{
					{
						SourceRef: &pbauth.Source_NamespaceRef{
							NamespaceRef: &pbauth.NamespaceRef{
								Namespace:           "web",
								PeerOrPartitionOrSg: &pbauth.NamespaceRef_Partition{Partition: "web-partition"},
							},
						},
					},
					{
						SourceRef: &pbauth.Source_Not{
							Not: &pbauth.Source{
								SourceRef: &pbauth.Source_NamespaceRef{
									NamespaceRef: &pbauth.NamespaceRef{
										Namespace: "intern",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	res, err = protojson.Marshal(intentions)
	require.NoError(t, err)
	fmt.Println(string(res))

	// Allow traffic on a certain path but deny on /admin path
	intentions = &pbauth.Intentions{
		Destination: &pbresource.Reference{
			Name: "api",
			Type: &pbresource.Type{Group: "auth", GroupVersion: "v1alpha1", Kind: "WorkloadIdentity"},
		},

		Action: pbauth.Action_ACTION_ALLOW,

		Rules: []*pbauth.Rule{
			{
				Sources: []*pbauth.Source{
					{
						SourceRef: &pbauth.Source_IdentityRef{
							IdentityRef: &pbauth.IdentityRef{
								Name:      "web",
								Namespace: "web",
							},
						},
					},
				},

				Permissions: []*pbauth.Permission{
					{
						Perm: &pbauth.Permission_NotRule{
							NotRule: &pbauth.PermissionRule{
								Http: &pbauth.HTTPPermissionRules{
									PathExact: "/admin",
								},
							},
						},
					},
					{
						Perm: &pbauth.Permission_Rule{
							Rule: &pbauth.PermissionRule{
								Http: &pbauth.HTTPPermissionRules{
									PathPrefix: "/",
								},
							},
						},
					},
				},
			},
		},
	}

	protojson.MarshalOptions{}
	res, err = protojson.Marshal(intentions)
	require.NoError(t, err)
	fmt.Println(string(res))
}
