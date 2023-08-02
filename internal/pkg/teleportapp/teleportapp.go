package teleportapp

import (
	"context"
	"fmt"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleportclient"
)

type TeleportApp struct {
	ctrlClient     client.Client
	logger         logr.Logger
	teleportClient *teleportclient.TeleportClient
}

type Config struct {
	CtrlClient     client.Client
	Logger         logr.Logger
	TeleportClient *teleportclient.TeleportClient
}

type AppConfig struct {
	InstallNamespace    string
	RegisterName        string
	ClusterName         string
	JoinToken           string
	IsManagementCluster bool
}

func New(config Config) (*TeleportApp, error) {
	return &TeleportApp{
		ctrlClient:     config.CtrlClient,
		logger:         config.Logger,
		teleportClient: config.TeleportClient,
	}, nil
}

func (t *TeleportApp) InstallApp(ctx context.Context, config *AppConfig) error {
	if err := t.ensureConfigmap(ctx, config); err != nil {
		return microerror.Mask(err)
	}

	if err := t.ensureApp(ctx, config); err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (t *TeleportApp) ensureConfigmap(ctx context.Context, config *AppConfig) error {
	logger := t.logger.WithValues("cluster", config.ClusterName)
	configMapName := fmt.Sprintf("%s-%s", config.ClusterName, t.teleportClient.AppName)

	name := key.GetConfigmapName(configMapName)

	data := map[string]string{
		"values": t.getConfigmapValues(config),
	}

	if t.teleportClient.TeleportVersion != "" {
		data["values"] = fmt.Sprintf("%steleportVersionOverride: %q", data["values"], t.teleportClient.TeleportVersion)
	}

	cm := corev1.ConfigMap{}
	err := t.ctrlClient.Get(ctx, client.ObjectKey{Name: name, Namespace: config.InstallNamespace}, &cm)
	if errors.IsNotFound(err) {
		logger.Info("Configmap does not exist.")
		cm := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: config.InstallNamespace,
				Labels: map[string]string{
					label.ManagedBy: key.TeleportOperatorLabelValue,
				},
			},
			Data: data,
		}

		err = t.ctrlClient.Create(ctx, &cm)
		if err != nil {
			return microerror.Mask(err)
		}

		logger.Info("Configmap created.")
		return nil
	}

	logger.Info("Configmap exists.")
	return nil
}

func (t *TeleportApp) ensureApp(ctx context.Context, config *AppConfig) error {
	logger := t.logger.WithValues("cluster", config.ClusterName)

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

	appName := key.GetAppName(config.ClusterName, t.teleportClient.AppName)
	appSpec := appv1alpha1.AppSpec{
		Catalog:    t.teleportClient.AppCatalog,
		KubeConfig: appSpecKubeConfig,
		Name:       t.teleportClient.AppName,
		Namespace:  key.TeleportKubeAppNamespace,
		UserConfig: appv1alpha1.AppSpecUserConfig{
			ConfigMap: appv1alpha1.AppSpecUserConfigConfigMap{
				Name:      key.GetConfigmapName(appName),
				Namespace: config.InstallNamespace,
			},
		},
		Version: t.teleportClient.AppVersion,
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
	err := t.ctrlClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: config.InstallNamespace}, &app)
	if errors.IsNotFound(err) {
		logger.Info("Installing teleport-kube-agent app...")
		if err = t.ctrlClient.Create(ctx, &desiredApp); err != nil {
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

func (t *TeleportApp) getConfigmapValues(config *AppConfig) string {
	dateTpl := `roles: "kube"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`
	return fmt.Sprintf(dateTpl, config.JoinToken, t.teleportClient.ProxyAddr, config.RegisterName)
}
