// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package xds

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/armon/go-metrics"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/hashicorp/go-hclog"

	external "github.com/hashicorp/consul/agent/grpc-external"
	"github.com/hashicorp/consul/agent/grpc-external/limiter"
	"github.com/hashicorp/consul/envoyextensions/xdscommon"
	proxysnapshot "github.com/hashicorp/consul/internal/mesh/proxy-snapshot"
)

type deltaSession struct {
	logger hclog.Logger
	stream ADSDeltaStream

	handlers map[string]*xDSDeltaType

	streamStartTime time.Time
	streamStartOnce sync.Once

	// set node
	node          *envoy_config_core_v3.Node
	proxyFeatures xdscommon.SupportedProxyFeatures

	// set watch
	stateCh     <-chan proxysnapshot.ProxySnapshot
	drainCh     limiter.SessionTerminatedChan
	watchCancel func()

	// snapshot exists
	proxySnapshot proxysnapshot.ProxySnapshot
	ready         bool // set to true after the first snapshot arrives

	// xds needs these
	nonce uint64 // xDS requires a unique nonce to correlate response/request pairs

	// resourceMap is the SoTW we are incrementally attempting to sync to envoy.
	//
	// type => name => proto
	resourceMap *xdscommon.IndexedResources

	// currentVersions is the xDS versioning represented by Resources.
	//
	// type => name => version (as consul knows right now)
	currentVersions map[string]map[string]string
}

func newDeltaSession(logger hclog.Logger, stream ADSDeltaStream) *deltaSession {
	session := &deltaSession{
		logger:          logger,
		stream:          stream,
		streamStartTime: time.Now(),
		resourceMap:     xdscommon.EmptyIndexedResources(),
		currentVersions: make(map[string]map[string]string),
	}

	// Configure handlers for each type of request we currently care about.
	session.handlers = map[string]*xDSDeltaType{
		xdscommon.ListenerType: newDeltaType(session.logger, stream, xdscommon.ListenerType, func() bool {
			return session.proxySnapshot.AllowEmptyListeners()
		}),
		xdscommon.RouteType: newDeltaType(session.logger, stream, xdscommon.RouteType, func() bool {
			return session.proxySnapshot.AllowEmptyRoutes()
		}),
		xdscommon.ClusterType: newDeltaType(session.logger, stream, xdscommon.ClusterType, func() bool {
			return session.proxySnapshot.AllowEmptyClusters()
		}),
		xdscommon.EndpointType: newDeltaType(session.logger, stream, xdscommon.EndpointType, nil),
		xdscommon.SecretType:   newDeltaType(session.logger, stream, xdscommon.SecretType, nil), // TODO allowEmptyFn
	}

	// Endpoints are stored within a Cluster (and Routes
	// are stored within a Listener) so whenever the
	// enclosing resource is updated the inner resource
	// list is cleared implicitly.
	//
	// When this happens we should update our local
	// representation of envoy state to force an update.
	//
	// see: https://github.com/envoyproxy/envoy/issues/13009
	session.handlers[xdscommon.ListenerType].deltaChild = &xDSDeltaChild{
		childType:     session.handlers[xdscommon.RouteType],
		childrenNames: make(map[string][]string),
	}
	session.handlers[xdscommon.ClusterType].deltaChild = &xDSDeltaChild{
		childType:     session.handlers[xdscommon.EndpointType],
		childrenNames: make(map[string][]string),
	}

	return session
}

func (s *deltaSession) updateLoggers(logger hclog.Logger) {
	s.logger = logger
	for _, handler := range s.handlers {
		handler.logger = logger
	}
}

func (s *deltaSession) CheckStreamACLs(srv *Server) error {
	// NOTE: it is not possible to call this method when proxySnapshot is unset.
	return srv.authorize(s.stream.Context(), s.proxySnapshot)
}

func (s *deltaSession) AcceptDiscoveryRequest(req *envoy_discovery_v3.DeltaDiscoveryRequest) (regen bool, err error) {
	logTraceRequest(s.logger, "Incremental xDS v3", req)

	if req.TypeUrl == "" {
		return false, status.Errorf(codes.InvalidArgument, "type URL is required for ADS")
	}

	if s.node == nil {
		if req.Node == nil {
			return false, fmt.Errorf("cannot continue without a node")
		}

		s.node = req.Node

		var err error
		s.proxyFeatures, err = xdscommon.DetermineSupportedProxyFeatures(s.node)
		if err != nil {
			return false, status.Errorf(codes.InvalidArgument, err.Error())
		}
	}

	// Once arriving here we are guaranteed both session.node
	// and session.proxyFeatures have been initialized.

	if handler, ok := s.handlers[req.TypeUrl]; ok {
		switch handler.Recv(req, s.proxyFeatures) {
		case deltaRecvNewSubscription:
			s.logger.Trace("subscribing to type", "typeUrl", req.TypeUrl)

		case deltaRecvResponseNack:
			s.logger.Trace("got nack response for type", "typeUrl", req.TypeUrl)

			// There is no reason to believe that generating new xDS resources from the same snapshot
			// would lead to an ACK from Envoy. Instead we continue to the top of this for loop and wait
			// for a new request or snapshot.
			return false, nil
		}
	}
	return true, nil
}

func (s *deltaSession) InitializeWatch(srv *Server) error {
	if s.node == nil {
		// This can't happen (tm) since stateCh is nil until after the first req
		// is received but lets not panic about it.
		return nil
	}

	nodeName := s.node.GetMetadata().GetFields()["node_name"].GetStringValue()
	if nodeName == "" {
		nodeName = srv.NodeName
	}

	// Start authentication process, we need the proxyID
	proxyID := newResourceIDFromEnvoyNode(s.node)

	// Start watching config for that proxy
	options, err := external.QueryOptionsFromContext(s.stream.Context())
	if err != nil {
		return status.Errorf(codes.Internal, "failed to watch proxy service: %s", err)
	}

	s.stateCh, s.drainCh, s.watchCancel, err = srv.ProxyWatcher.Watch(proxyID, nodeName, options.Token)
	switch {
	case errors.Is(err, limiter.ErrCapacityReached):
		return errOverwhelmed
	case err != nil:
		return status.Errorf(codes.Internal, "failed to watch proxy: %s", err)
	}

	// enhance future logs
	s.updateLoggers(s.logger.With("service_id", proxyID.Name))

	s.logger.Trace("watching proxy, pending initial proxycfg snapshot for xDS")

	// Now wait for the config so we can check ACL
	return nil
}

func (s *deltaSession) InstallSnapshot(srv *Server, cs proxysnapshot.ProxySnapshot) error {
	if cs == nil {
		return nil // should not happen but it doesn't hurt to ignore it
	}
	s.proxySnapshot = cs

	newRes, err := getEnvoyConfiguration(s.proxySnapshot, s.logger, srv.CfgFetcher)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to generate all xDS resources from the snapshot: %v", err)
	}

	// index and hash the xDS structures
	newResourceMap := xdscommon.IndexResources(s.logger, newRes)

	if srv.ResourceMapMutateFn != nil {
		srv.ResourceMapMutateFn(newResourceMap)
	}

	if newResourceMap, err = srv.applyEnvoyExtensions(newResourceMap, s.proxySnapshot, s.node); err != nil {
		// err is already the result of calling status.Errorf
		return err
	}

	if err := populateChildIndexMap(newResourceMap); err != nil {
		return status.Errorf(codes.Unavailable, "failed to index xDS resource versions: %v", err)
	}

	newVersions, err := computeResourceVersions(newResourceMap)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to compute xDS resource versions: %v", err)
	}

	if !s.ready {
		// Got config, try to authenticate next.

		// Upgrade the logger
		loggerName := s.proxySnapshot.LoggerName()
		if loggerName != "" {
			s.updateLoggers(s.logger.Named(loggerName))
		}

		s.logger.Trace("Got initial config snapshot")
	}

	s.resourceMap = newResourceMap
	s.currentVersions = newVersions
	s.ready = true

	return nil
}

func (s *deltaSession) UpdateEnvoyIfNecessary() error {
	s.logger.Trace("Invoking all xDS resource handlers and sending changed data if there are any")
	s.streamStartOnce.Do(func() {
		metrics.MeasureSince([]string{"xds", "server", "streamStart"}, s.streamStartTime)
	})

	for _, op := range xDSUpdateOrder {
		if op.TypeUrl == xdscommon.ListenerType || op.TypeUrl == xdscommon.RouteType {
			if clusterHandler := s.handlers[xdscommon.ClusterType]; clusterHandler.registered && len(clusterHandler.pendingUpdates) > 0 {
				s.logger.Trace("Skipping delta computation for resource because there are dependent updates pending",
					"typeUrl", op.TypeUrl, "dependent", xdscommon.ClusterType)

				// Receiving an ACK from Envoy will unblock the select statement above,
				// and re-trigger an attempt to send these skipped updates.
				break
			}
			if endpointHandler := s.handlers[xdscommon.EndpointType]; endpointHandler.registered && len(endpointHandler.pendingUpdates) > 0 {
				s.logger.Trace("Skipping delta computation for resource because there are dependent updates pending",
					"typeUrl", op.TypeUrl, "dependent", xdscommon.EndpointType)

				// Receiving an ACK from Envoy will unblock the select statement above,
				// and re-trigger an attempt to send these skipped updates.
				break
			}
		}
		err, _ := s.handlers[op.TypeUrl].SendIfNew(s.currentVersions[op.TypeUrl], s.resourceMap, &s.nonce, op.Upsert, op.Remove)
		if err != nil {
			return status.Errorf(codes.Unavailable,
				"failed to send %sreply for type %q: %v",
				op.errorLogNameReplyPrefix(),
				op.TypeUrl, err)
		}
	}
	return nil
}

func (s *deltaSession) Stop() {
	// Note that in this case we _intend_ the defer to only be triggered when
	// this whole process method ends (i.e. when streaming RPC aborts) not at
	// the end of the current loop iteration. We have to do it in the loop
	// here since we can't start watching until we get to this state in the
	// state machine.
	if s.watchCancel != nil {
		s.watchCancel()
	}
}
