package controller

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"google.golang.org/protobuf/proto"

	oldctrl "github.com/hashicorp/consul/agent/consul/controller"
	"github.com/hashicorp/consul/internal/resource"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

type Manager struct {
	logger hclog.Logger
	client pbresource.ResourceServiceClient

	mu          sync.Mutex
	running     bool
	controllers []Controller
}

type ManagerDeps struct {
	Logger hclog.Logger
	Client pbresource.ResourceServiceClient
}

func NewManager(deps ManagerDeps) *Manager {
	return &Manager{
		logger: deps.Logger,
		client: deps.Client,
	}
}

func (m *Manager) AddController(ctrl Controller) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		panic("cannot add Controllers to a running Manager")
	}

	m.controllers = append(m.controllers, ctrl)
}

func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()

	for _, b := range m.controllers {
		logger := b.logger
		if logger == nil {
			logger = m.logger
		}
		ctrl := &controller{
			client:      m.client,
			logger:      logger,
			queue:       oldctrl.RunWorkQueue[Request](ctx, 1*time.Second, 2*time.Second),
			managedType: b.managedType,
			reconciler:  b.reconciler,
			watches:     b.watches,
		}
		go ctrl.run(ctx)
	}
}

func ForType(managedType *pbresource.Type) Controller {
	return Controller{managedType: managedType}
}

type Controller struct {
	managedType *pbresource.Type
	reconciler  Reconciler
	logger      hclog.Logger
	watches     []watch
}

type watch struct {
	watchedType *pbresource.Type
	mapper      DependancyMapper
}

func (b Controller) WithReconciler(rec Reconciler) Controller {
	b.reconciler = rec
	return b
}

func (b Controller) WithLogger(logger hclog.Logger) Controller {
	b.logger = logger
	return b
}

type DependancyMapper func(ctx context.Context, rt Runtime, res *pbresource.Resource) ([]Request, error)

func (b Controller) WithWatch(watchedType *pbresource.Type, mapper DependancyMapper) Controller {
	b.watches = append(b.watches, watch{watchedType, mapper})
	return b
}

func MapOwner(_ context.Context, _ Runtime, res *pbresource.Resource) ([]Request, error) {
	var reqs []Request
	if res.Owner != nil {
		reqs = append(reqs, Request{ID: res.Owner})
	}
	return reqs, nil
}

type Runtime struct {
	Client pbresource.ResourceServiceClient
	Logger hclog.Logger
}

type Request struct {
	ID *pbresource.ID
}

type Reconciler interface {
	Reconcile(ctx context.Context, rt Runtime, req Request) error
}

type controller struct {
	client pbresource.ResourceServiceClient
	logger hclog.Logger
	queue  oldctrl.WorkQueue[Request]

	managedType *pbresource.Type
	reconciler  Reconciler
	watches     []watch
}

func (c *controller) run(ctx context.Context) {
	go c.watchManagedType(ctx)

	for _, w := range c.watches {
		go c.watchDep(ctx, w)
	}

	for {
		req, done := c.queue.Get()
		if done {
			return
		}

		rt := Runtime{
			Client: c.client,
			Logger: c.logger,
		}

		// TODO: panic handling.
		if err := c.reconciler.Reconcile(ctx, rt, req); err == nil {
			c.queue.AddRateLimited(req)
		} else {
			c.queue.Done(req)
		}
	}
}

func (c *controller) watchDep(ctx context.Context, w watch) {
	c.logger.Info("watching dep", "dep", w.watchedType)

	watch, err := c.client.WatchList(ctx, &pbresource.WatchListRequest{
		Type: w.watchedType,
		Tenancy: &pbresource.Tenancy{
			Partition: resource.Wildcard,
			PeerName:  resource.Wildcard,
			Namespace: resource.Wildcard,
		},
	})
	if err != nil {
		// TODO: What should we do in this situation?
		c.logger.Error("failed to begin watch", "error", err)
		return
	}

	for {
		event, err := watch.Recv()
		if err != nil {
			// TODO: What should we do in this situation?
			c.logger.Error("failed to receive from watch", "error", err)
			return
		}

		reqs, err := w.mapper(ctx, Runtime{Client: c.client, Logger: c.logger}, event.Resource)
		if err != nil {
			// TODO: Should we retry if this happens?
			c.logger.Error("mapper failed", "error", err)
			continue
		}

		for _, req := range reqs {
			if proto.Equal(req.ID.Type, c.managedType) {
				c.queue.Add(req)
			} else {
				c.logger.Error("mapper returned wrong type", "expected", c.managedType, "got", req.ID.Type)
			}
		}
	}
}

func (c *controller) watchManagedType(ctx context.Context) {
	watch, err := c.client.WatchList(ctx, &pbresource.WatchListRequest{
		Type: c.managedType,
		Tenancy: &pbresource.Tenancy{
			Partition: resource.Wildcard,
			PeerName:  resource.Wildcard,
			Namespace: resource.Wildcard,
		},
	})
	if err != nil {
		// TODO: What should we do in this situation?
		c.logger.Error("failed to begin watch", "error", err)
		return
	}

	for {
		event, err := watch.Recv()
		if err != nil {
			// TODO: What should we do in this situation?
			c.logger.Error("failed to receive from watch", "error", err)
			return
		}
		c.queue.Add(Request{ID: event.Resource.Id})
	}
}
