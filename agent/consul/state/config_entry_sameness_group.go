// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package state

import (
	"github.com/absolutelightning/go-memdb"
	"github.com/hashicorp/consul/agent/configentry"
	"github.com/hashicorp/consul/agent/structs"
)

// GetSamenessGroup returns a SamenessGroupConfigEntry from the state
// store using the provided parameters.
func (s *Store) GetSamenessGroup(ws memdb.WatchSet,
	name string,
	overrides map[configentry.KindName]structs.ConfigEntry,
	partition string) (uint64, *structs.SamenessGroupConfigEntry, error) {
	tx := s.db.ReadTxn()
	defer tx.Abort()

	return getSamenessGroupConfigEntryTxn(tx, ws, name, overrides, partition)
}
