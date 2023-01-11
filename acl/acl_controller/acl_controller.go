package acl_controller

import (
	"context"

	"github.com/hashicorp/consul/agent/consul/controller"
	"github.com/hashicorp/consul/agent/consul/state"
	"github.com/hashicorp/consul/agent/consul/stream"

	"github.com/hashicorp/go-hclog"
)

type aclReconciler struct {
	logger hclog.Logger
}

func (r aclReconciler) Reconcile(ctx context.Context, req controller.Request) error {
	return nil
}

func NewACLController(publisher state.EventPublisher, logger hclog.Logger) controller.Controller {
	reconciler := aclReconciler{
		logger: logger,
	}
	return controller.New(publisher, reconciler).Subscribe(
		&stream.SubscribeRequest{
			Topic:   state.EventTopicACL,
			Subject: stream.SubjectWildcard,
		},
	)
}
