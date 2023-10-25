// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package catalogv2

import (
	"strings"

	"github.com/hashicorp/consul/testing/deployer/topology"
	"github.com/stretchr/testify/require"
)

func clusterPrefixForUpstream(u *topology.Upstream) string {
	if u.Peer == "" {
		if u.ID.PartitionOrDefault() == "default" {
			return strings.Join([]string{u.PortName, u.ID.Name, u.ID.Namespace, u.Cluster, "internal"}, ".")
		} else {
			return strings.Join([]string{u.PortName, u.ID.Name, u.ID.Namespace, u.ID.Partition, u.Cluster, "internal-v1"}, ".")
		}
	} else {
		return strings.Join([]string{u.ID.Name, u.ID.Namespace, u.Peer, "external"}, ".")
	}
}

func clusterPrefix(port string, svcID topology.ServiceID, cluster string) string {
	if svcID.PartitionOrDefault() == "default" {
		return strings.Join([]string{port, svcID.Name, svcID.Namespace, cluster, "internal"}, ".")
	} else {
		return strings.Join([]string{port, svcID.Name, svcID.Namespace, svcID.Partition, cluster, "internal-v1"}, ".")
	}
}

func assertTrafficSplit(t require.TestingT, nameCounts map[string]int, expect map[string]int, epsilon int) {
	require.Len(t, nameCounts, len(expect))
	for name, expectCount := range expect {
		gotCount, ok := nameCounts[name]
		require.True(t, ok)
		require.InEpsilon(t, expectCount, gotCount, float64(epsilon),
			"expected %q side of split to have %d requests not %d (e=%d)",
			name, expectCount, gotCount, epsilon,
		)
	}
}
