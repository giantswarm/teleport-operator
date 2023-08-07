package teleport

import (
	"context"

	"github.com/giantswarm/microerror"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func RemoveFinalizer(ctx context.Context, config *TeleportConfig) error {
	patchHelper, err := patch.NewHelper(config.Cluster, config.CtrlClient)
	if err != nil {
		return errors.WithStack(err)
	}

	controllerutil.RemoveFinalizer(config.Cluster, key.TeleportOperatorFinalizer)
	if err := patchHelper.Patch(ctx, config.Cluster); err != nil {
		config.Log.Error(err, "failed to remove finalizer")
		return microerror.Mask(client.IgnoreNotFound(err))
	}
	config.Log.Info("Finalizer removed", "finalizer_name", key.TeleportOperatorFinalizer)
	return nil
}

func AddFinalizer(ctx context.Context, config *TeleportConfig) error {
	patchHelper, err := patch.NewHelper(config.Cluster, config.CtrlClient)
	if err != nil {
		return errors.WithStack(err)
	}

	controllerutil.AddFinalizer(config.Cluster, key.TeleportOperatorFinalizer)
	if err := patchHelper.Patch(ctx, config.Cluster); err != nil {
		config.Log.Error(err, "failed to add finalizer")
		return microerror.Mask(client.IgnoreNotFound(err))
	}
	config.Log.Info("Finalizer added", "finalizer_name", key.TeleportOperatorFinalizer)
	return nil
}
