package demo

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul/internal/controller"
	"github.com/hashicorp/consul/proto-public/pbresource"
)

func ArtistController(logger hclog.Logger) controller.Controller {
	return controller.
		ForType(TypeV2Artist).
		WithWatch(TypeV2Album, controller.MapOwner).
		WithReconciler(&artistReconciler{}).
		WithLogger(logger)
}

type artistReconciler struct{}

func (r *artistReconciler) Reconcile(ctx context.Context, rt controller.Runtime, req controller.Request) error {
	rsp, err := rt.Client.Read(ctx, &pbresource.ReadRequest{Id: req.ID})
	if err != nil {
		return err
	}

	listRsp, err := rt.Client.List(ctx, &pbresource.ListRequest{
		Type:       TypeV2Album,
		Tenancy:    req.ID.Tenancy,
		NamePrefix: fmt.Sprintf("%s/", req.ID.Name),
	})
	if err != nil {
		return err
	}
	if len(listRsp.Resources) >= 5 {
		return nil
	}

	for i := 0; i < 5; i++ {
		album, err := GenerateV2Album(rsp.Resource.Id)
		if err != nil {
			rt.Logger.Error("failed to generate album", "error", err)
			return err
		}

		if _, err := rt.Client.Write(ctx, &pbresource.WriteRequest{Resource: album}); err != nil {
			rt.Logger.Error("failed to write album", "error", err)
			return err
		}
	}

	return nil
}
