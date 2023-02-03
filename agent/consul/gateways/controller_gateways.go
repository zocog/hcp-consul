package gateways

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-memdb"

	"github.com/hashicorp/consul/acl"
	"github.com/hashicorp/consul/agent/configentry"
	"github.com/hashicorp/consul/agent/consul/controller"
	"github.com/hashicorp/consul/agent/consul/discoverychain"
	"github.com/hashicorp/consul/agent/consul/fsm"
	"github.com/hashicorp/consul/agent/consul/state"
	"github.com/hashicorp/consul/agent/consul/stream"
	"github.com/hashicorp/consul/agent/structs"
)

type Updater struct {
	UpdateWithStatus func(entry structs.ControlledConfigEntry) error
	Update           func(entry structs.ConfigEntry) error
	Delete           func(entry structs.ConfigEntry) error
}

// gatewayMeta embeds both a BoundAPIGateway and its corresponding APIGateway.
// This is used when binding routes to a gateway to ensure that a route's protocol (e.g. http)
// matches the protocol of the listener it wants to bind to. The binding modifies the
// "bound" gateway, but relies on the "gateway" to determine the protocol of the listener.
type gatewayMeta struct {
	// BoundGateway is the bound-api-gateway config entry for a given gateway.
	BoundGateway *structs.BoundAPIGatewayConfigEntry
	// Gateway is the api-gateway config entry for the gateway.
	Gateway *structs.APIGatewayConfigEntry
}

// getAllGatewayMeta returns a pre-constructed list of all valid gateway and state
// tuples based on the state coming from the store. Any gateway that does not have
// a current bound state will be filtered out.
func getAllGatewayMeta(store *state.Store) ([]*gatewayMeta, error) {
	_, gateways, err := store.ConfigEntriesByKind(nil, structs.APIGateway, acl.WildcardEnterpriseMeta())
	if err != nil {
		return nil, err
	}
	_, boundGateways, err := store.ConfigEntriesByKind(nil, structs.BoundAPIGateway, acl.WildcardEnterpriseMeta())
	if err != nil {
		return nil, err
	}

	meta := make([]*gatewayMeta, 0, len(boundGateways))
	for _, b := range boundGateways {
		bound := b.(*structs.BoundAPIGatewayConfigEntry)
		for _, g := range gateways {
			gateway := g.(*structs.APIGatewayConfigEntry)
			if bound.References(gateway) {
				meta = append(meta, &gatewayMeta{
					BoundGateway: bound,
					Gateway:      gateway,
				})
				break
			}
		}
	}
	return meta, nil
}

// updateRouteBinding takes a parent resource reference and a BoundRoute and
// modifies the listeners on the BoundAPIGateway config entry in GatewayMeta
// to reflect the binding of the route to the gateway.
//
// If the reference is not valid or the route's protocol does not match the
// targeted listener's protocol, a mapping of parent references to associated
// errors is returned.
func (g *gatewayMeta) updateRouteBinding(refs []structs.ResourceReference, route structs.BoundRoute) (bool, map[structs.ResourceReference]error) {
	if g.BoundGateway == nil || g.Gateway == nil {
		return false, nil
	}

	didUpdate := false
	errors := make(map[structs.ResourceReference]error)

	if len(g.BoundGateway.Listeners) == 0 {
		for _, ref := range refs {
			errors[ref] = fmt.Errorf("route cannot bind because gateway has no listeners")
		}
		return false, errors
	}

	for i, listener := range g.BoundGateway.Listeners {
		routeRef := structs.ResourceReference{
			Kind:           route.GetKind(),
			Name:           route.GetName(),
			EnterpriseMeta: *route.GetEnterpriseMeta(),
		}
		// Unbind to handle any stale route references.
		didUnbind := listener.UnbindRoute(routeRef)
		if didUnbind {
			didUpdate = true
		}
		g.BoundGateway.Listeners[i] = listener

		for _, ref := range refs {
			didBind, err := g.bindRoute(ref, route)
			if err != nil {
				errors[ref] = err
			}
			if didBind {
				didUpdate = true
			}
		}
	}

	return didUpdate, errors
}

// bindRoute takes a parent reference and a route and attempts to bind the route to the
// bound gateway in the gatewayMeta struct. It returns true if the route was bound and
// false if it was not. If the route fails to bind, an error is returned.
//
// Binding logic binds a route to one or more listeners on the Bound gateway.
// For a route to successfully bind it must:
//   - have a parent reference to the gateway
//   - have a parent reference with a section name matching the name of a listener
//     on the gateway. If the section name is `""`, the route will be bound to all
//     listeners on the gateway whose protocol matches the route's protocol.
//   - have a protocol that matches the protocol of the listener it is being bound to.
func (g *gatewayMeta) bindRoute(ref structs.ResourceReference, route structs.BoundRoute) (bool, error) {
	if g.BoundGateway == nil || g.Gateway == nil {
		return false, fmt.Errorf("gateway cannot be found")
	}

	if ref.Kind != structs.APIGateway || g.Gateway.Name != ref.Name || !g.Gateway.EnterpriseMeta.IsSame(&ref.EnterpriseMeta) {
		return false, nil
	}

	if len(g.BoundGateway.Listeners) == 0 {
		return false, fmt.Errorf("route cannot bind because gateway has no listeners")
	}

	didBind := false
	for _, listener := range g.Gateway.Listeners {
		// A route with a section name of "" is bound to all listeners on the gateway.
		if listener.Name != ref.SectionName && ref.SectionName != "" {
			continue
		}

		if listener.Protocol == route.GetProtocol() {
			routeRef := structs.ResourceReference{
				Kind:           route.GetKind(),
				Name:           route.GetName(),
				EnterpriseMeta: *route.GetEnterpriseMeta(),
			}
			i, boundListener := g.boundListenerByName(listener.Name)
			if boundListener != nil && boundListener.BindRoute(routeRef) {
				didBind = true
				g.BoundGateway.Listeners[i] = *boundListener
			}
		} else if ref.SectionName != "" {
			// Failure to bind to a specific listener is an error
			return false, fmt.Errorf("failed to bind route %s to gateway %s: listener %s is not a %s listener", route.GetName(), g.Gateway.Name, listener.Name, route.GetProtocol())
		}
	}

	if !didBind {
		return didBind, fmt.Errorf("failed to bind route %s to gateway %s: no valid listener has name '%s' and uses %s protocol", route.GetName(), g.Gateway.Name, ref.SectionName, route.GetProtocol())
	}

	return didBind, nil
}

// unbindRoute takes a route and unbinds it from all of the listeners on a gateway.
// It returns true if the route was unbound and false if it was not.
func (g *gatewayMeta) unbindRoute(route structs.ResourceReference) bool {
	if g.BoundGateway == nil {
		return false
	}

	didUnbind := false
	for i, listener := range g.BoundGateway.Listeners {
		if listener.UnbindRoute(route) {
			didUnbind = true
			g.BoundGateway.Listeners[i] = listener
		}
	}

	return didUnbind
}

func (g *gatewayMeta) boundListenerByName(name string) (int, *structs.BoundAPIGatewayListener) {
	for i, listener := range g.BoundGateway.Listeners {
		if listener.Name == name {
			return i, &listener
		}
	}
	return -1, nil
}

func (g *gatewayMeta) checkCertificates(store *state.Store) (map[structs.ResourceReference]error, error) {
	certificateErrors := map[structs.ResourceReference]error{}
	for i, listener := range g.Gateway.Listeners {
		bound := g.BoundGateway.Listeners[i]
		for _, ref := range listener.TLS.Certificates {
			_, certificate, err := store.ConfigEntry(nil, ref.Kind, ref.Name, &ref.EnterpriseMeta)
			if err != nil {
				return nil, err
			}
			if certificate == nil {
				certificateErrors[ref] = errors.New("certificate not found")
			} else {
				bound.Certificates = append(bound.Certificates, ref)
			}
		}
	}
	return certificateErrors, nil
}

func (g *gatewayMeta) checkConflicts() (structs.ControlledConfigEntry, bool) {
	now := time.Now()
	updater := structs.NewStatusUpdater(g.Gateway)
	for i, listener := range g.BoundGateway.Listeners {
		protocol := g.Gateway.Listeners[i].Protocol
		switch protocol {
		case structs.ListenerProtocolTCP:
			if len(listener.Routes) > 1 {
				updater.SetCondition(structs.Condition{
					Type:   "Conflicted",
					Status: "True",
					Reason: "RouteConflict",
					Resource: &structs.ResourceReference{
						Kind:           structs.APIGateway,
						Name:           g.Gateway.Name,
						SectionName:    listener.Name,
						EnterpriseMeta: g.Gateway.EnterpriseMeta,
					},
					Message:            "TCP-based listeners currently only support binding a single route",
					LastTransitionTime: &now,
				})
			}
			continue
		}
		updater.SetCondition(structs.Condition{
			Type:   "Conflicted",
			Status: "False",
			Reason: "NoConflict",
			Resource: &structs.ResourceReference{
				Kind:           structs.APIGateway,
				Name:           g.Gateway.Name,
				SectionName:    listener.Name,
				EnterpriseMeta: g.Gateway.EnterpriseMeta,
			},
			Message:            "listener has no route conflicts",
			LastTransitionTime: &now,
		})
	}

	toUpdate, shouldUpdate := updater.UpdateEntry()
	return toUpdate, shouldUpdate
}

func ensureInitializedMeta(gateway *structs.APIGatewayConfigEntry, bound structs.ConfigEntry) *gatewayMeta {
	var b *structs.BoundAPIGatewayConfigEntry
	if bound == nil {
		b = &structs.BoundAPIGatewayConfigEntry{
			Kind:           structs.BoundAPIGateway,
			Name:           gateway.Name,
			EnterpriseMeta: gateway.EnterpriseMeta,
		}
	} else {
		b = bound.(*structs.BoundAPIGatewayConfigEntry).DeepCopy()
	}

	// we just clear out the bound state here since we recalculate it entirely
	// in the gateway control loop
	listeners := []structs.BoundAPIGatewayListener{}
	for _, listener := range gateway.Listeners {
		listeners = append(listeners, structs.BoundAPIGatewayListener{
			Name: listener.Name,
		})
	}
	b.Listeners = listeners
	return &gatewayMeta{
		BoundGateway: b,
		Gateway:      gateway,
	}
}

func stateIsDirty(initial, final *structs.BoundAPIGatewayConfigEntry) bool {
	initialListeners := map[string]structs.BoundAPIGatewayListener{}
	for _, listener := range initial.Listeners {
		initialListeners[listener.Name] = listener
	}
	finalListeners := map[string]structs.BoundAPIGatewayListener{}
	for _, listener := range final.Listeners {
		finalListeners[listener.Name] = listener
	}

	if len(initialListeners) != len(finalListeners) {
		return true
	}

	for name, initialListener := range initialListeners {
		finalListener, found := finalListeners[name]
		if !found {
			return true
		}
		if !initialListener.IsSame(finalListener) {
			return true
		}
	}
	return false
}

// referenceSet stores an O(1) accessible set of ResourceReference objects.
type referenceSet = map[structs.ResourceReference]any

// gatewayRefs maps a gateway kind/name to a set of resource references.
type gatewayRefs = map[configentry.KindName][]structs.ResourceReference

// BindRoutesToGateways takes a slice of bound API gateways and a variadic number of routes.
// It iterates over the parent references for each route. These parents are gateways the
// route should be bound to. If the parent matches a bound gateway, the route is bound to the
// gateway. Otherwise, the route is unbound from the gateway if it was previously bound.
//
// The function returns a list of references to the modified BoundAPIGatewayConfigEntry objects,
// a map of resource references to errors that occurred when they were attempted to be
// bound to a gateway.
func BindRoutesToGateways(gateways []*gatewayMeta, routes ...structs.BoundRoute) ([]*structs.BoundAPIGatewayConfigEntry, []structs.ResourceReference, map[structs.ResourceReference]error) {
	boundRefs := []structs.ResourceReference{}
	modified := make([]*structs.BoundAPIGatewayConfigEntry, 0, len(gateways))

	// errored stores the errors from events where a resource reference failed to bind to a gateway.
	errored := make(map[structs.ResourceReference]error)

	for _, route := range routes {
		parentRefs, gatewayRefs := getReferences(route)
		routeRef := structs.ResourceReference{
			Kind:           route.GetKind(),
			Name:           route.GetName(),
			EnterpriseMeta: *route.GetEnterpriseMeta(),
		}

		// Iterate over all BoundAPIGateway config entries and try to bind them to the route if they are a parent.
		for _, gateway := range gateways {
			references, routeReferencesGateway := gatewayRefs[configentry.NewKindNameForEntry(gateway.BoundGateway)]
			if routeReferencesGateway {
				didUpdate, errors := gateway.updateRouteBinding(references, route)
				if didUpdate {
					modified = append(modified, gateway.BoundGateway)
				}
				for ref, err := range errors {
					errored[ref] = err
				}
				for _, ref := range references {
					delete(parentRefs, ref)
					// this ref successfully bound, add it to the set that we'll update the
					// status for
					if _, found := errored[ref]; !found {
						boundRefs = append(boundRefs, references...)
					}
				}
			} else if gateway.unbindRoute(routeRef) {
				modified = append(modified, gateway.BoundGateway)
			}
		}

		// Add all references that aren't bound at this point to the error set.
		for reference := range parentRefs {
			errored[reference] = errors.New("invalid reference to missing parent")
		}
	}

	return modified, boundRefs, errored
}

// getReferences returns a set of all the resource references for a given route as well as
// a map of gateway kind/name to a list of resource references for that gateway.
func getReferences(route structs.BoundRoute) (referenceSet, gatewayRefs) {
	parentRefs := make(referenceSet)
	gatewayRefs := make(gatewayRefs)
	for _, ref := range route.GetParents() {
		parentRefs[ref] = struct{}{}
		kindName := configentry.NewKindName(structs.BoundAPIGateway, ref.Name, &ref.EnterpriseMeta)
		gatewayRefs[kindName] = append(gatewayRefs[kindName], ref)
	}

	return parentRefs, gatewayRefs
}

func requestToResourceRef(req controller.Request) structs.ResourceReference {
	ref := structs.ResourceReference{
		Kind: req.Kind,
		Name: req.Name,
	}
	if req.Meta != nil {
		ref.EnterpriseMeta = *req.Meta
	}
	return ref
}

// RemoveGateway sets the route status for the given gateway to be unbound if it should be bound
func RemoveGateway(gateway structs.ResourceReference, entries ...structs.BoundRoute) []structs.ControlledConfigEntry {
	now := time.Now()
	modified := []structs.ControlledConfigEntry{}
	for _, route := range entries {
		updater := structs.NewStatusUpdater(route)
		for _, parent := range route.GetParents() {
			if parent.Kind == gateway.Kind && parent.Name == parent.Name && parent.EnterpriseMeta.IsSame(&gateway.EnterpriseMeta) {
				updater.SetCondition(structs.Condition{
					Type:               "Bound",
					Status:             "False",
					Reason:             "GatewayNotFound",
					Message:            "gateway was not found",
					Resource:           &parent,
					LastTransitionTime: &now,
				})
			}
		}
		if toUpdate, shouldUpdate := updater.UpdateEntry(); shouldUpdate {
			modified = append(modified, toUpdate)
		}
	}
	return modified
}

// RemoveRoute unbinds the route from the given gateways, returning the list of gateways that were modified.
func RemoveRoute(route structs.ResourceReference, entries ...*gatewayMeta) []*gatewayMeta {
	modified := []*gatewayMeta{}

	for _, entry := range entries {
		if entry.unbindRoute(route) {
			modified = append(modified, entry)
		}
	}

	return modified
}

type apiGatewayReconciler struct {
	fsm        *fsm.FSM
	logger     hclog.Logger
	updater    *Updater
	controller controller.Controller
}

func (r apiGatewayReconciler) Reconcile(ctx context.Context, req controller.Request) error {
	// We do this in a single threaded way to avoid race conditions around setting
	// shared state. In our current out-of-repo code, this is handled via a global
	// lock on our shared store, but this makes it so we don't have to deal with lock
	// contention, and instead just work with a single control loop.
	switch req.Kind {
	case structs.APIGateway:
		return reconcileEntry(r.fsm.State(), ctx, req, r.reconcileGateway, r.cleanupGateway)
	case structs.BoundAPIGateway:
		return reconcileEntry(r.fsm.State(), ctx, req, r.reconcileBoundGateway, r.cleanupBoundGateway)
	case structs.HTTPRoute:
		return reconcileEntry(r.fsm.State(), ctx, req, func(ctx context.Context, req controller.Request, store *state.Store, route *structs.HTTPRouteConfigEntry) error {
			return r.reconcileRoute(ctx, req, store, route)
		}, r.cleanupRoute)
	case structs.TCPRoute:
		return reconcileEntry(r.fsm.State(), ctx, req, func(ctx context.Context, req controller.Request, store *state.Store, route *structs.TCPRouteConfigEntry) error {
			return r.reconcileRoute(ctx, req, store, route)
		}, r.cleanupRoute)
	case structs.InlineCertificate:
		return r.enqueueCertificateReferencedGateways(r.fsm.State(), ctx, req)
	default:
		return nil
	}
}

func reconcileEntry[T structs.ControlledConfigEntry](store *state.Store, ctx context.Context, req controller.Request, reconciler func(ctx context.Context, req controller.Request, store *state.Store, entry T) error, cleaner func(ctx context.Context, req controller.Request, store *state.Store) error) error {
	_, entry, err := store.ConfigEntry(nil, req.Kind, req.Name, req.Meta)
	if err != nil {
		return err
	}
	if entry == nil {
		return cleaner(ctx, req, store)
	}
	return reconciler(ctx, req, store, entry.(T))
}

func (r apiGatewayReconciler) enqueueCertificateReferencedGateways(store *state.Store, ctx context.Context, req controller.Request) error {
	_, entries, err := store.ConfigEntriesByKind(nil, structs.APIGateway, acl.WildcardEnterpriseMeta())
	if err != nil {
		return err
	}
	requests := []controller.Request{}
	for _, entry := range entries {
		gateway := entry.(*structs.APIGatewayConfigEntry)
		for _, listener := range gateway.Listeners {
			for _, certificate := range listener.TLS.Certificates {
				if certificate.IsSame(&structs.ResourceReference{
					Kind:           req.Kind,
					Name:           req.Name,
					EnterpriseMeta: *req.Meta,
				}) {
					requests = append(requests, controller.Request{
						Kind: structs.APIGateway,
						Name: gateway.Name,
						Meta: &gateway.EnterpriseMeta,
					})
				}
			}
		}
	}
	r.controller.Enqueue(requests...)
	return nil
}

func (r apiGatewayReconciler) cleanupBoundGateway(ctx context.Context, req controller.Request, store *state.Store) error {
	_, httpRoutes, err := store.ConfigEntriesByKind(nil, structs.HTTPRoute, acl.WildcardEnterpriseMeta())
	if err != nil {
		return err
	}
	_, tcpRoutes, err := store.ConfigEntriesByKind(nil, structs.TCPRoute, acl.WildcardEnterpriseMeta())
	if err != nil {
		return err
	}

	routes := make([]structs.BoundRoute, 0, len(tcpRoutes)+len(httpRoutes))
	for _, route := range httpRoutes {
		routes = append(routes, route.(*structs.HTTPRouteConfigEntry))
	}
	for _, route := range tcpRoutes {
		routes = append(routes, route.(*structs.TCPRouteConfigEntry))
	}

	r.logger.Debug("cleaning up deleted bound gateway object", "request", req)
	resource := requestToResourceRef(req)
	resource.Kind = structs.APIGateway

	for _, toUpdate := range RemoveGateway(resource, routes...) {
		if err := r.updater.Update(toUpdate); err != nil {
			return err
		}
	}
	return nil
}

func (r apiGatewayReconciler) reconcileBoundGateway(ctx context.Context, req controller.Request, store *state.Store, bound *structs.BoundAPIGatewayConfigEntry) error {
	// this reconciler handles orphaned bound gateways at startup, it just checks to make sure there's still an existing gateway, and if not, it deletes the bound gateway
	r.logger.Debug("got bound gateway reconcile call", "request", req)
	_, gateway, err := store.ConfigEntry(nil, structs.APIGateway, req.Name, req.Meta)
	if err != nil {
		return err
	}
	if gateway == nil {
		// delete the bound gateway
		return r.updater.Delete(bound)
	}
	return nil
}

func (r apiGatewayReconciler) cleanupGateway(ctx context.Context, req controller.Request, store *state.Store) error {
	_, bound, err := store.ConfigEntry(nil, structs.BoundAPIGateway, req.Name, req.Meta)
	if err != nil {
		return err
	}
	return r.updater.Delete(bound)
}

func (r apiGatewayReconciler) reconcileGateway(ctx context.Context, req controller.Request, store *state.Store, gateway *structs.APIGatewayConfigEntry) error {
	now := time.Now()

	r.logger.Debug("got gateway reconcile call", "request", req)

	_, httpRoutes, err := store.ConfigEntriesByKind(nil, structs.HTTPRoute, acl.WildcardEnterpriseMeta())
	if err != nil {
		return err
	}
	_, tcpRoutes, err := store.ConfigEntriesByKind(nil, structs.TCPRoute, acl.WildcardEnterpriseMeta())
	if err != nil {
		return err
	}

	routes := make([]structs.BoundRoute, 0, len(tcpRoutes)+len(httpRoutes))
	for _, route := range httpRoutes {
		routes = append(routes, route.(*structs.HTTPRouteConfigEntry))
	}
	for _, route := range tcpRoutes {
		routes = append(routes, route.(*structs.TCPRouteConfigEntry))
	}

	updater := structs.NewStatusUpdater(gateway)
	// we clear out the initial status conditions since we're doing a full update
	// of this gateway's status
	updater.ClearConditions()

	// construct the tuple we'll be working on to update state
	_, bound, err := store.ConfigEntry(nil, structs.BoundAPIGateway, req.Name, req.Meta)
	if err != nil {
		return err
	}
	meta := ensureInitializedMeta(gateway, bound)
	certificateErrors, err := meta.checkCertificates(store)
	if err != nil {
		return err
	}

	for ref, err := range certificateErrors {
		updater.SetCondition(structs.Condition{
			Type:               "Accepted",
			Status:             "False",
			Reason:             "InvalidCertificate",
			Message:            err.Error(),
			Resource:           &ref,
			LastTransitionTime: &now,
		})
	}
	if len(certificateErrors) > 0 {
		updater.SetCondition(structs.Condition{
			Type:               "Accepted",
			Status:             "False",
			Reason:             "InvalidCertificates",
			Message:            "gateway references invalid certificates",
			LastTransitionTime: &now,
		})
	} else {
		updater.SetCondition(structs.Condition{
			Type:               "Accepted",
			Status:             "True",
			Reason:             "Accepted",
			Message:            "gateway is valid",
			LastTransitionTime: &now,
		})
	}

	// now we bind all of the routes we can
	updatedRoutes := []structs.ControlledConfigEntry{}
	for _, route := range routes {
		routeUpdater := structs.NewStatusUpdater(route)
		_, boundRefs, bindErrors := BindRoutesToGateways([]*gatewayMeta{meta}, route)
		// unset the old gateway binding in case it's stale
		for _, parent := range route.GetParents() {
			if parent.Kind == gateway.Kind && parent.Name == parent.Name && parent.EnterpriseMeta.IsSame(&gateway.EnterpriseMeta) {
				routeUpdater.RemoveCondition(structs.Condition{
					Type:     "Bound",
					Resource: &parent,
				})
			}
		}
		for _, ref := range boundRefs {
			routeUpdater.SetCondition(structs.Condition{
				Type:               "Bound",
				Status:             "True",
				Reason:             "Bound",
				Resource:           &ref,
				Message:            "successfully bound route",
				LastTransitionTime: &now,
			})
		}
		for reference, err := range bindErrors {
			routeUpdater.SetCondition(structs.Condition{
				Type:               "Bound",
				Status:             "False",
				Reason:             "FailedToBind",
				Resource:           &reference,
				Message:            err.Error(),
				LastTransitionTime: &now,
			})
		}
		if entry, updated := routeUpdater.UpdateEntry(); updated {
			updatedRoutes = append(updatedRoutes, entry)
		}
	}

	// first check for gateway conflicts
	for i, listener := range meta.BoundGateway.Listeners {
		protocol := meta.Gateway.Listeners[i].Protocol
		switch protocol {
		case structs.ListenerProtocolTCP:
			if len(listener.Routes) > 1 {
				updater.SetCondition(structs.Condition{
					Type:    "Conflicted",
					Status:  "True",
					Reason:  "RouteConflict",
					Message: "TCP-based listeners currently only support binding a single route",
					Resource: &structs.ResourceReference{
						Kind:           structs.APIGateway,
						Name:           meta.Gateway.Name,
						SectionName:    listener.Name,
						EnterpriseMeta: meta.Gateway.EnterpriseMeta,
					},
					LastTransitionTime: &now,
				})
				continue
			}
		}
		updater.SetCondition(structs.Condition{
			Type:   "Conflicted",
			Status: "False",
			Reason: "NoConflict",
			Resource: &structs.ResourceReference{
				Kind:           structs.APIGateway,
				Name:           meta.Gateway.Name,
				SectionName:    listener.Name,
				EnterpriseMeta: meta.Gateway.EnterpriseMeta,
			},
			Message:            "listener has no route conflicts",
			LastTransitionTime: &now,
		})
	}

	// now check if we need to update the gateway status
	if toUpdate, shouldUpdate := updater.UpdateEntry(); shouldUpdate {
		r.logger.Debug("persisting gateway status", "gateway", gateway)
		if err := r.updater.UpdateWithStatus(toUpdate); err != nil {
			r.logger.Error("error persisting gateway status", "error", err)
			return err
		}
	}

	// next update route statuses
	for _, toUpdate := range updatedRoutes {
		r.logger.Debug("persisting route status", "route", toUpdate)
		if err := r.updater.UpdateWithStatus(toUpdate); err != nil {
			r.logger.Error("error persisting route status", "error", err)
			return err
		}
	}

	// now update the bound state if it changed
	if bound == nil || stateIsDirty(bound.(*structs.BoundAPIGatewayConfigEntry), meta.BoundGateway) {
		r.logger.Debug("persisting gateway state", "state", meta.BoundGateway)
		if err := r.updater.Update(meta.BoundGateway); err != nil {
			r.logger.Error("error persisting state", "error", err)
			return err
		}
	}

	return nil
}

func (r apiGatewayReconciler) cleanupRoute(ctx context.Context, req controller.Request, store *state.Store) error {
	meta, err := getAllGatewayMeta(store)
	if err != nil {
		return err
	}

	r.logger.Debug("cleaning up deleted route object", "request", req)
	for _, toUpdate := range RemoveRoute(requestToResourceRef(req), meta...) {
		if err := r.updater.Update(toUpdate.BoundGateway); err != nil {
			return err
		}
	}
	r.controller.RemoveTrigger(req)
	return nil
}

// Reconcile reconciles Route config entries.
func (r apiGatewayReconciler) reconcileRoute(ctx context.Context, req controller.Request, store *state.Store, route structs.BoundRoute) error {
	now := time.Now()

	meta, err := getAllGatewayMeta(store)
	if err != nil {
		return err
	}

	r.logger.Debug("got route reconcile call", "request", req)

	updater := structs.NewStatusUpdater(route)
	// we clear out the initial status conditions since we're doing a full update
	// of this route's status
	updater.ClearConditions()

	ws := memdb.NewWatchSet()
	ws.Add(store.AbandonCh())

	finalize := func(modifiedGateways []*structs.BoundAPIGatewayConfigEntry) error {
		// first update any gateway statuses that are now in conflict
		for _, gateway := range meta {
			toUpdate, shouldUpdate := gateway.checkConflicts()
			if shouldUpdate {
				r.logger.Debug("persisting gateway status", "gateway", gateway)
				if err := r.updater.UpdateWithStatus(toUpdate); err != nil {
					r.logger.Error("error persisting gateway", "error", gateway)
					return err
				}
			}
		}

		// next update the route status
		if toUpdate, shouldUpdate := updater.UpdateEntry(); shouldUpdate {
			r.logger.Debug("persisting route status", "route", route)
			if err := r.updater.UpdateWithStatus(toUpdate); err != nil {
				r.logger.Error("error persisting route", "error", route)
				return err
			}
		}

		// now update all of the state values
		for _, state := range modifiedGateways {
			r.logger.Debug("persisting gateway state", "state", state)
			if err := r.updater.Update(state); err != nil {
				r.logger.Error("error persisting state", "error", err)
				return err
			}
		}

		return nil
	}

	var triggerOnce sync.Once
	validTargets := true
	for _, service := range route.GetTargetedServices() {
		_, chainSet, err := store.ReadDiscoveryChainConfigEntries(ws, service.Name, &service.EnterpriseMeta)
		if err != nil {
			return err
		}
		// trigger a watch since we now need to check when the discovery chain gets updated
		triggerOnce.Do(func() {
			r.controller.AddTrigger(req, ws.WatchCtx)
		})

		if chainSet.IsEmpty() {
			updater.SetCondition(structs.Condition{
				Type:               "Accepted",
				Status:             "False",
				Reason:             "InvalidDiscoveryChain",
				Message:            "service does not exist",
				LastTransitionTime: &now,
			})
			continue
		}

		// make sure that we can actually compile a discovery chain based on this route
		// the main check is to make sure that all of the protocols align
		chain, err := discoverychain.Compile(discoverychain.CompileRequest{
			ServiceName:           service.Name,
			EvaluateInNamespace:   service.NamespaceOrDefault(),
			EvaluateInPartition:   service.PartitionOrDefault(),
			EvaluateInDatacenter:  "dc1",           // just mock out a fake dc since we're just checking for compilation errors
			EvaluateInTrustDomain: "consul.domain", // just mock out a fake trust domain since we're just checking for compilation errors
			Entries:               chainSet,
		})
		if err != nil {
			// we only really need to return the first error for an invalid
			// discovery chain, but we still want to set watches on everything in the
			// store
			if validTargets {
				updater.SetCondition(structs.Condition{
					Type:               "Accepted",
					Status:             "False",
					Reason:             "InvalidDiscoveryChain",
					Message:            err.Error(),
					LastTransitionTime: &now,
				})
				validTargets = false
			}
			continue
		}

		if chain.Protocol != string(route.GetProtocol()) {
			if validTargets {
				updater.SetCondition(structs.Condition{
					Type:               "Accepted",
					Status:             "False",
					Reason:             "InvalidDiscoveryChain",
					Message:            "route protocol does not match targeted service protocol",
					LastTransitionTime: &now,
				})
				validTargets = false
			}
			continue
		}

		// this makes sure we don't override an already set status
		if validTargets {
			updater.SetCondition(structs.Condition{
				Type:               "Accepted",
				Status:             "True",
				Reason:             "Accepted",
				Message:            "route is valid",
				LastTransitionTime: &now,
			})
		}
	}
	if len(route.GetTargetedServices()) == 0 {
		updater.SetCondition(structs.Condition{
			Type:               "Accepted",
			Status:             "False",
			Reason:             "NoUpstreamServicesTargeted",
			Message:            "route must target at least one upstream service",
			LastTransitionTime: &now,
		})
		validTargets = false
	}
	if !validTargets {
		// we return early, but need to make sure we're removed from all referencing
		// gateways and our status is updated properly
		updated := []*structs.BoundAPIGatewayConfigEntry{}
		for _, toUpdate := range RemoveRoute(requestToResourceRef(req), meta...) {
			updated = append(updated, toUpdate.BoundGateway)
		}
		return finalize(updated)
	}

	r.logger.Debug("binding routes to gateway")

	// the route is valid, attempt to bind it to all gateways
	modifiedGateways, boundRefs, bindErrors := BindRoutesToGateways(meta, route)
	if err != nil {
		return err
	}

	// set the status of the references that are bound
	for _, ref := range boundRefs {
		updater.SetCondition(structs.Condition{
			Type:               "Bound",
			Status:             "True",
			Reason:             "Bound",
			Resource:           &ref,
			Message:            "successfully bound route",
			LastTransitionTime: &now,
		})
	}

	// set any binding errors
	for reference, err := range bindErrors {
		updater.SetCondition(structs.Condition{
			Type:               "Bound",
			Status:             "False",
			Reason:             "FailedToBind",
			Resource:           &reference,
			Message:            err.Error(),
			LastTransitionTime: &now,
		})
	}

	return finalize(modifiedGateways)
}

func NewAPIGatewayController(fsm *fsm.FSM, publisher state.EventPublisher, updater *Updater, logger hclog.Logger) controller.Controller {
	reconciler := apiGatewayReconciler{
		fsm:     fsm,
		logger:  logger,
		updater: updater,
	}
	reconciler.controller = controller.New(publisher, reconciler)
	return reconciler.controller.Subscribe(
		&stream.SubscribeRequest{
			Topic:   state.EventTopicAPIGateway,
			Subject: stream.SubjectWildcard,
		},
	).Subscribe(
		&stream.SubscribeRequest{
			Topic:   state.EventTopicHTTPRoute,
			Subject: stream.SubjectWildcard,
		},
	).Subscribe(
		&stream.SubscribeRequest{
			Topic:   state.EventTopicTCPRoute,
			Subject: stream.SubjectWildcard,
		},
	).Subscribe(
		&stream.SubscribeRequest{
			Topic:   state.EventTopicBoundAPIGateway,
			Subject: stream.SubjectWildcard,
		},
	).Subscribe(
		&stream.SubscribeRequest{
			Topic:   state.EventTopicInlineCertificate,
			Subject: stream.SubjectWildcard,
		})
}
