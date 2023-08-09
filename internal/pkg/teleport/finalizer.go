package teleport

import (
	"context"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func RemoveFinalizer(ctx context.Context, log logr.Logger, cluster *capi.Cluster, ctrlClient client.Client) error {
	patchHelper, err := patch.NewHelper(cluster, ctrlClient)
	if err != nil {
		return errors.WithStack(err)
	}

	controllerutil.RemoveFinalizer(cluster, key.TeleportOperatorFinalizer)
	if err := patchHelper.Patch(ctx, cluster); err != nil {
		log.Error(err, "failed to remove finalizer")
		return microerror.Mask(client.IgnoreNotFound(err))
	}
	log.Info("Removed finalizer", "finalizer_name", key.TeleportOperatorFinalizer)
	return nil
}

func AddFinalizer(ctx context.Context, log logr.Logger, cluster *capi.Cluster, ctrlClient client.Client) error {
	patchHelper, err := patch.NewHelper(cluster, ctrlClient)
	if err != nil {
		return errors.WithStack(err)
	}

	controllerutil.AddFinalizer(cluster, key.TeleportOperatorFinalizer)
	if err := patchHelper.Patch(ctx, cluster); err != nil {
		log.Error(err, "failed to add finalizer")
		return microerror.Mask(client.IgnoreNotFound(err))
	}
	log.Info("Added finalizer", "finalizer_name", key.TeleportOperatorFinalizer)
	return nil
}
