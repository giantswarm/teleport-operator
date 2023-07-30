package teleportapp

import (
	"context"
	"fmt"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/kubectl-gs/pkg/project"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

type TeleportApp struct {
	ctrlClient        client.Client
	logger            logr.Logger
	teleportProxyAddr string
	teleportVersion   string
	appCatalog        string
	appVersion        string
	appName           string
}

type Config struct {
	CtrlClient        client.Client
	Logger            logr.Logger
	TeleportProxyAddr string
	TeleportVersion   string
	AppName           string
	AppVersion        string
	AppCatalog        string
}

func New(config Config) (*TeleportApp, error) {
	// if config.CtrlClient == nil {
	// 	return nil, microerror.Maskf(invalidConfigError, "%T.CtrlClient must not be empty", config)
	// }
	// if config.TeleportProxyAddr == "" {
	// 	return nil, microerror.Maskf(invalidConfigError, "%T.TeleportProxyAddr must not be empty", config)
	// }

	return &TeleportApp{
		ctrlClient:        config.CtrlClient,
		logger:            config.Logger,
		teleportProxyAddr: config.TeleportProxyAddr,
		teleportVersion:   config.TeleportVersion,
		appName:           config.AppName,
		appVersion:        config.AppVersion,
		appCatalog:        config.AppCatalog,
	}, nil
}

func (t *TeleportApp) InstallApp(ctx context.Context, namespace string, registerName string, clusterName string, joinToken string, isManagementCluster bool) error {
	if err := t.ensureConfigmap(ctx, namespace, registerName, clusterName, joinToken); err != nil {
		return microerror.Mask(err)
	}

	if err := t.ensureApp(ctx, namespace, registerName, clusterName, isManagementCluster); err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (t *TeleportApp) ensureConfigmap(ctx context.Context, namespace string, registerName string, clusterName string, joinToken string) error {
	logger := t.logger.WithValues("cluster", clusterName)
	configMapName := fmt.Sprintf("%s-%s", clusterName, t.appName)

	name := key.ConfigmapName(configMapName)

	dateTpl := `roles: "kube"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`
	data := map[string]string{
		"values": fmt.Sprintf(dateTpl, joinToken, t.teleportProxyAddr, registerName),
	}

	if t.teleportVersion != "" {
		data["values"] = fmt.Sprintf("%steleportVersionOverride: %q", data["values"], t.teleportVersion)
	}

	cm := corev1.ConfigMap{}
	err := t.ctrlClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &cm)
	if errors.IsNotFound(err) {
		logger.Info("Configmap does not exist.")
		cm := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					label.ManagedBy: project.Name(),
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

func (t *TeleportApp) ensureApp(ctx context.Context, appNamespace string, registerName string, clusterName string, isManagementCluster bool) error {
	logger := t.logger.WithValues("cluster", clusterName)

	appSpecKubeConfig := appv1alpha1.AppSpecKubeConfig{
		InCluster: isManagementCluster,
	}

	if !isManagementCluster {
		appSpecKubeConfig.Context = appv1alpha1.AppSpecKubeConfigContext{
			Name: registerName,
		}
		appSpecKubeConfig.Secret = appv1alpha1.AppSpecKubeConfigSecret{
			Name:      fmt.Sprintf("%s-kubeconfig", clusterName),
			Namespace: appNamespace,
		}
	}

	appName := fmt.Sprintf("%s-%s", clusterName, t.appName)
	appSpec := appv1alpha1.AppSpec{
		Catalog:    t.appCatalog,
		KubeConfig: appSpecKubeConfig,
		Name:       t.appName,
		Namespace:  "kube-system",
		UserConfig: appv1alpha1.AppSpecUserConfig{
			ConfigMap: appv1alpha1.AppSpecUserConfigConfigMap{
				Name:      key.ConfigmapName(appName),
				Namespace: appNamespace,
			},
		},
		Version: t.appVersion,
	}

	desiredApp := appv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: appNamespace,
			Labels: map[string]string{
				label.ManagedBy: project.Name(),
				label.Cluster:   clusterName,
			},
		},
		Spec: appSpec,
	}
	if isManagementCluster {
		desiredApp.Labels[label.AppOperatorVersion] = "0.0.0"
	}

	app := appv1alpha1.App{}
	err := t.ctrlClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: appNamespace}, &app)
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

	logger.Info("App already exists, but not registered with teleport yet.")
	return nil
}
