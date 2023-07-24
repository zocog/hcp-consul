// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package consul

import (
	"context"
	"fmt"
	"time"

	"github.com/armon/go-metrics"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"google.golang.org/grpc/metadata"

	"github.com/hashicorp/consul/agent/configentry"
	"github.com/hashicorp/consul/agent/structs"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

func diffConfigEntries(local []structs.ConfigEntry, remote []structs.ConfigEntry, lastRemoteIndex uint64) ([]structs.ConfigEntry, []structs.ConfigEntry) {
	configentry.SortSlice(local)
	configentry.SortSlice(remote)

	var (
		deletions []structs.ConfigEntry
		updates   []structs.ConfigEntry
		localIdx  int
		remoteIdx int
	)
	for localIdx, remoteIdx = 0, 0; localIdx < len(local) && remoteIdx < len(remote); {
		if configentry.EqualID(local[localIdx], remote[remoteIdx]) {
			// config is in both the local and remote state - need to check raft indices
			if remote[remoteIdx].GetRaftIndex().ModifyIndex > lastRemoteIndex {
				updates = append(updates, remote[remoteIdx])
			}
			// increment both indices when equal
			localIdx += 1
			remoteIdx += 1
		} else if configentry.Less(local[localIdx], remote[remoteIdx]) {
			// config no longer in remoted state - needs deleting
			deletions = append(deletions, local[localIdx])

			// increment just the local index
			localIdx += 1
		} else {
			// local state doesn't have this config - needs updating
			updates = append(updates, remote[remoteIdx])

			// increment just the remote index
			remoteIdx += 1
		}
	}

	for ; localIdx < len(local); localIdx += 1 {
		deletions = append(deletions, local[localIdx])
	}

	for ; remoteIdx < len(remote); remoteIdx += 1 {
		updates = append(updates, remote[remoteIdx])
	}

	return deletions, updates
}

func (s *Server) reconcileLocalConfig(ctx context.Context, configs []structs.ConfigEntry, op structs.ConfigEntryOp) (bool, error) {
	ticker := time.NewTicker(time.Second / time.Duration(s.config.ConfigReplicationApplyLimit))
	defer ticker.Stop()

	rpcServiceMethod := "ConfigEntry.Apply"
	if op == structs.ConfigEntryDelete || op == structs.ConfigEntryDeleteCAS {
		rpcServiceMethod = "ConfigEntry.Delete"
	}

	var merr error
	for i, entry := range configs {
		// Exported services only apply to the primary datacenter.
		if entry.GetKind() == structs.ExportedServices {
			continue
		}
		req := structs.ConfigEntryRequest{
			Op:         op,
			Datacenter: s.config.Datacenter,
			Entry:      entry,
		}

		_, err := s.leaderRaftApply(rpcServiceMethod, structs.ConfigEntryRequestType, &req)
		if err != nil {
			merr = multierror.Append(merr, fmt.Errorf("Failed to apply config entry %s: %w", op, err))
		}

		if i < len(configs)-1 {
			select {
			case <-ctx.Done():
				return true, nil
			case <-ticker.C:
				// do nothing - ready for the next batch
			}
		}
	}

	return false, merr
}

func (s *Server) fetchConfigEntries(lastRemoteIndex uint64) (*structs.IndexedGenericConfigEntries, error) {
	defer metrics.MeasureSince([]string{"leader", "replication", "config-entries", "fetch"}, time.Now())

	req := structs.ConfigEntryListAllRequest{
		Datacenter: s.config.PrimaryDatacenter,
		Kinds:      structs.AllConfigEntryKinds,
		QueryOptions: structs.QueryOptions{
			AllowStale:    true,
			MinQueryIndex: lastRemoteIndex,
			Token:         s.tokens.ReplicationToken(),
		},
		EnterpriseMeta: *s.replicationEnterpriseMeta(),
	}

	var response structs.IndexedGenericConfigEntries
	if err := s.RPC(context.Background(), "ConfigEntry.ListAll", &req, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// We need to do watch lists. We can't order resources based on version or generation.
func (s *Server) fetchResources(t *pbresource.Type, local bool) (*pbresource.ListResponse, error) {
	ctx := context.Background()
	client := s.insecureResourceServiceClient
	if !local {
		ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{
			"x-consul-datacenter":     s.config.PrimaryDatacenter,
			"x-consul-token":          s.tokens.ReplicationToken(),
			"x-consul-bypass-forward": "true",
		}))
		client = s.secureResourceServiceClient
	}

	resp, err := client.List(ctx, &pbresource.ListRequest{
		Type: t,
		Tenancy: &pbresource.Tenancy{
			Partition: "default",
			Namespace: "*",
			PeerName:  "local",
		},
	})
	return resp, err
}

func (s *Server) replicateConfig(ctx context.Context, lastRemoteIndex uint64, logger hclog.Logger) (uint64, bool, error) {
	remote, err := s.fetchConfigEntries(lastRemoteIndex)
	if err != nil {
		return 0, false, fmt.Errorf("failed to retrieve remote config entries: %v", err)
	}

	logger.Debug("finished fetching config entries", "amount", len(remote.Entries))

	// Need to check if we should be stopping. This will be common as the fetching process is a blocking
	// RPC which could have been hanging around for a long time and during that time leadership could
	// have been lost.
	select {
	case <-ctx.Done():
		return 0, true, nil
	default:
		// do nothing
	}

	// Measure everything after the remote query, which can block for long
	// periods of time. This metric is a good measure of how expensive the
	// replication process is.
	defer metrics.MeasureSince([]string{"leader", "replication", "config", "apply"}, time.Now())

	_, local, err := s.fsm.State().ConfigEntries(nil, s.replicationEnterpriseMeta())
	if err != nil {
		return 0, false, fmt.Errorf("failed to retrieve local config entries: %v", err)
	}

	// If the remote index ever goes backwards, it's a good indication that
	// the remote side was rebuilt and we should do a full sync since we
	// can't make any assumptions about what's going on.
	//
	// Resetting lastRemoteIndex to 0 will work because we never consider local
	// raft indices. Instead we compare the raft modify index in the response object
	// with the lastRemoteIndex (only when we already have a config entry of the same kind/name)
	// to determine if an update is needed. Resetting lastRemoteIndex to 0 then has the affect
	// of making us think all the local state is out of date and any matching entries should
	// still be updated.
	//
	// The lastRemoteIndex is not used when the entry exists either only in the local state or
	// only in the remote state. In those situations we need to either delete it or create it.
	if remote.QueryMeta.Index < lastRemoteIndex {
		logger.Warn("Config Entry replication remote index moved backwards, forcing a full Config Entry sync",
			"from", lastRemoteIndex,
			"to", remote.QueryMeta.Index,
		)
		lastRemoteIndex = 0
	}

	logger.Debug("Config Entry replication",
		"local", len(local),
		"remote", len(remote.Entries),
	)
	// Calculate the changes required to bring the state into sync and then
	// apply them.
	deletions, updates := diffConfigEntries(local, remote.Entries, lastRemoteIndex)

	logger.Debug("Config Entry replication",
		"deletions", len(deletions),
		"updates", len(updates),
	)

	var merr error
	if len(deletions) > 0 {
		logger.Debug("Deleting local config entries",
			"deletions", len(deletions),
		)

		exit, err := s.reconcileLocalConfig(ctx, deletions, structs.ConfigEntryDelete)
		if exit {
			return 0, true, nil
		}
		if err != nil {
			merr = multierror.Append(merr, err)
		} else {
			logger.Debug("Config Entry replication - finished deletions")
		}
	}

	if len(updates) > 0 {
		logger.Debug("Updating local config entries",
			"updates", len(updates),
		)
		exit, err := s.reconcileLocalConfig(ctx, updates, structs.ConfigEntryUpsert)
		if exit {
			return 0, true, nil
		}
		if err != nil {
			merr = multierror.Append(merr, err)
		} else {
			logger.Debug("Config Entry replication - finished updates")
		}
	}

	if merr != nil {
		return 0, false, merr
	}

	// Return the index we got back from the remote side, since we've synced
	// up with the remote state as of that index.
	return remote.QueryMeta.Index, false, nil
}

type RemoteGenerations map[resourceId]string

// TODO
func (s *Server) replicateResource(ctx context.Context, t *pbresource.Type, remoteVersions RemoteGenerations, logger hclog.Logger) (RemoteGenerations, bool, error) {
	remote, err := s.fetchResources(t, false)
	if err != nil {
		return remoteVersions, false, fmt.Errorf("failed to retrieve remote resources: %v", err)
	}

	logger.Debug("finished fetching resources", "amount", len(remote.Resources))

	// Measure everything after the remote query, which can block for long
	// periods of time. This metric is a good measure of how expensive the
	// replication process is.
	defer metrics.MeasureSince([]string{"leader", "replication", "resources", "apply"}, time.Now())

	local, err := s.fetchResources(t, true)
	if err != nil {
		return remoteVersions, false, fmt.Errorf("failed to retrieve local resources: %v", err)
	}

	logger.Debug("Resource replication",
		"local", len(local.Resources),
		"remote", len(remote.Resources),
	)
	// Calculate the changes required to bring the state into sync and then
	// apply them.
	deletions, updates, remoteVersions := diffResources(local.Resources, remote.Resources, remoteVersions)

	logger.Debug("Resource replication",
		"deletions", len(deletions),
		"updates", len(updates),
		"remoteVersions", remoteVersions,
	)

	ctx = metadata.NewOutgoingContext(ctx, metadata.New(map[string]string{
		"x-consul-token":          s.tokens.ReplicationToken(),
		"x-consul-bypass-forward": "true",
	}))

	var merr error
	if len(deletions) > 0 {
		logger.Debug("Deleting local resources",
			"deletions", len(deletions),
		)

		for _, r := range deletions {
			_, err := s.insecureResourceServiceClient.Delete(ctx, &pbresource.DeleteRequest{
				Id: r.Id,
			})
			if err != nil {
				merr = multierror.Append(merr, err)
			} else {
				logger.Debug("Resource replication - finished deletions")
			}
		}
	}

	if len(updates) > 0 {
		logger.Debug("Updating local resources",
			"updates", len(updates),
		)
		for _, r := range updates {
			r.Status = nil
			_, err := s.insecureResourceServiceClient.Write(ctx, &pbresource.WriteRequest{
				Resource: r,
			})
			if err != nil {
				merr = multierror.Append(merr, err)
			} else {
				logger.Debug("Resource replication - finished updates")
			}
		}
	}

	if merr != nil {
		return remoteVersions, false, merr
	}

	// Return the index we got back from the remote side, since we've synced
	// up with the remote state as of that index.
	return remoteVersions, false, nil
}

type resourceId struct {
	name      string
	namespace string
}

func diffResources(local []*pbresource.Resource, remote []*pbresource.Resource, remoteGenerations RemoteGenerations) ([]*pbresource.Resource, []*pbresource.Resource, RemoteGenerations) {
	var deletions []*pbresource.Resource
	var updates []*pbresource.Resource

	seen := make(map[resourceId]struct{})
	for i := range remote {
		r := remote[i]
		id := resourceId{
			name:      r.Id.Name,
			namespace: r.GetId().GetTenancy().GetNamespace(),
		}
		seen[id] = struct{}{}
		// if we didn't know about it or the version changes, use it.
		if oldGeneration, ok := remoteGenerations[id]; !ok || oldGeneration != r.Generation {
			remoteGenerations[id] = r.Generation
			r.Status = nil
			r.Version = ""
			r.Generation = ""
			updates = append(updates, r)
		}
	}

	for i := range local {
		l := local[i]
		id := resourceId{
			name:      l.Id.Name,
			namespace: l.GetId().GetTenancy().GetNamespace(),
		}

		// if it doesn't exist on the remote, remove it.
		if _, ok := seen[id]; !ok {
			deletions = append(deletions, l)
		}
	}

	for id := range remoteGenerations {
		if _, ok := seen[id]; !ok {
			delete(remoteGenerations, id)
		}
	}

	return deletions, updates, remoteGenerations
}
