package teleport

import (
	"context"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsKubeAgentAppInstalled(ctx context.Context, ctrlClient client.Client, clusterName string, installNamespace string) (bool, error) {
	appName := key.GetAppName(clusterName, t.SecretConfig.AppName)
	app := appv1alpha1.App{}

	err := ctrlClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: installNamespace}, &app)
	if apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, microerror.Mask(err)
	}

	return true, nil
}

func (t *Teleport) InstallKubeAgentApp(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, registerName string, installNamespace string, isManagementCluster bool) error {
	appSpecKubeConfig := appv1alpha1.AppSpecKubeConfig{
		InCluster: isManagementCluster,
	}

	if !isManagementCluster {
		appSpecKubeConfig.Context = appv1alpha1.AppSpecKubeConfigContext{
			Name: registerName,
		}
		appSpecKubeConfig.Secret = appv1alpha1.AppSpecKubeConfigSecret{
			Name:      key.GetAppSpecKubeConfigSecretName(clusterName),
			Namespace: installNamespace,
		}
	}

	appName := key.GetAppName(clusterName, t.SecretConfig.AppName)
	appSpec := appv1alpha1.AppSpec{
		Catalog:    t.SecretConfig.AppCatalog,
		KubeConfig: appSpecKubeConfig,
		Name:       t.SecretConfig.AppName,
		Namespace:  key.TeleportKubeAppNamespace,
		UserConfig: appv1alpha1.AppSpecUserConfig{
			ConfigMap: appv1alpha1.AppSpecUserConfigConfigMap{
				Name:      key.GetConfigmapName(clusterName, t.SecretConfig.AppName),
				Namespace: installNamespace,
			},
		},
		Version: t.SecretConfig.AppVersion,
	}

	desiredApp := appv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: installNamespace,
			Labels: map[string]string{
				label.ManagedBy: key.TeleportOperatorLabelValue,
				label.Cluster:   clusterName,
			},
		},
		Spec: appSpec,
	}
	if isManagementCluster {
		desiredApp.Labels[label.AppOperatorVersion] = key.AppOperatorVersion
	}

	if err := ctrlClient.Create(ctx, &desiredApp); err != nil {
		return microerror.Mask(err)
	}

	log.Info("Installed teleport kube agent app", "appName", appName)
	return nil
}
