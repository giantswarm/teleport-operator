package teleport

import (
	"context"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsKubeAgentAppInstalled(ctx context.Context, config *TeleportConfig) (bool, error) {
	appName := key.GetAppName(config.Cluster.Name, t.SecretConfig.AppName)
	app := appv1alpha1.App{}

	err := config.CtrlClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: config.InstallNamespace}, &app)
	if apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, microerror.Mask(err)
	}

	return true, nil
}

func (t *Teleport) InstallKubeAgentApp(ctx context.Context, config *TeleportConfig) error {
	appSpecKubeConfig := appv1alpha1.AppSpecKubeConfig{
		InCluster: config.IsManagementCluster,
	}

	if !config.IsManagementCluster {
		appSpecKubeConfig.Context = appv1alpha1.AppSpecKubeConfigContext{
			Name: config.RegisterName,
		}
		appSpecKubeConfig.Secret = appv1alpha1.AppSpecKubeConfigSecret{
			Name:      key.GetAppSpecKubeConfigSecretName(config.Cluster.Name),
			Namespace: config.InstallNamespace,
		}
	}

	appName := key.GetAppName(config.Cluster.Name, t.SecretConfig.AppName)
	appSpec := appv1alpha1.AppSpec{
		Catalog:    t.SecretConfig.AppCatalog,
		KubeConfig: appSpecKubeConfig,
		Name:       t.SecretConfig.AppName,
		Namespace:  key.TeleportKubeAppNamespace,
		UserConfig: appv1alpha1.AppSpecUserConfig{
			ConfigMap: appv1alpha1.AppSpecUserConfigConfigMap{
				Name:      key.GetConfigmapName(config.Cluster.Name, t.SecretConfig.AppName),
				Namespace: config.InstallNamespace,
			},
		},
		Version: t.SecretConfig.AppVersion,
	}

	desiredApp := appv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: config.InstallNamespace,
			Labels: map[string]string{
				label.ManagedBy: key.TeleportOperatorLabelValue,
				label.Cluster:   config.Cluster.Name,
			},
		},
		Spec: appSpec,
	}
	if config.IsManagementCluster {
		desiredApp.Labels[label.AppOperatorVersion] = key.AppOperatorVersion
	}

	if err := config.CtrlClient.Create(ctx, &desiredApp); err != nil {
		return microerror.Mask(err)
	}

	config.Log.Info("Installed teleport kube agent app", "appName", appName)
	return nil
}
