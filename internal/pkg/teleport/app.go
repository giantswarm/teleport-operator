package teleport

import (
	"context"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type InstallAppConfig struct {
	InstallNamespace    string
	RegisterName        string
	ClusterName         string
	JoinToken           string
	IsManagementCluster bool
}

func (t *Teleport) InstallApp(ctx context.Context, config *InstallAppConfig) error {
	if err := t.EnsureConfigmap(ctx, config); err != nil {
		return microerror.Mask(err)
	}

	if err := t.ensureApp(ctx, config); err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (t *Teleport) ensureApp(ctx context.Context, config *InstallAppConfig) error {
	logger := t.Logger.WithValues("cluster", config.ClusterName)

	appSpecKubeConfig := appv1alpha1.AppSpecKubeConfig{
		InCluster: config.IsManagementCluster,
	}

	if !config.IsManagementCluster {
		appSpecKubeConfig.Context = appv1alpha1.AppSpecKubeConfigContext{
			Name: config.RegisterName,
		}
		appSpecKubeConfig.Secret = appv1alpha1.AppSpecKubeConfigSecret{
			Name:      key.GetAppSpecKubeConfigSecretName(config.ClusterName),
			Namespace: config.InstallNamespace,
		}
	}

	appName := key.GetAppName(config.ClusterName, t.Secret.AppName)
	appSpec := appv1alpha1.AppSpec{
		Catalog:    t.Secret.AppCatalog,
		KubeConfig: appSpecKubeConfig,
		Name:       t.Secret.AppName,
		Namespace:  key.TeleportKubeAppNamespace,
		UserConfig: appv1alpha1.AppSpecUserConfig{
			ConfigMap: appv1alpha1.AppSpecUserConfigConfigMap{
				Name:      key.GetConfigmapName(config.ClusterName, t.Secret.AppName),
				Namespace: config.InstallNamespace,
			},
		},
		Version: t.Secret.AppVersion,
	}

	desiredApp := appv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: config.InstallNamespace,
			Labels: map[string]string{
				label.ManagedBy: key.TeleportOperatorLabelValue,
				label.Cluster:   config.ClusterName,
			},
		},
		Spec: appSpec,
	}
	if config.IsManagementCluster {
		desiredApp.Labels[label.AppOperatorVersion] = key.AppOperatorVersion
	}

	app := appv1alpha1.App{}
	err := t.CtrlClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: config.InstallNamespace}, &app)
	if errors.IsNotFound(err) {
		logger.Info("Installing teleport-kube-agent app...")
		if err = t.CtrlClient.Create(ctx, &desiredApp); err != nil {
			return microerror.Mask(err)
		}
		logger.Info("App created.")
		return nil
	} else if err != nil {
		return microerror.Mask(err)
	}

	logger.Info("App already exists.")
	return nil
}
