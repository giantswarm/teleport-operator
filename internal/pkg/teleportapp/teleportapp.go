package teleportapp

import (
	"context"
	"fmt"
	"reflect"

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

func (t *TeleportApp) EnsureApp(ctx context.Context, namespace string, clusterName string, managementClusterName string, token string, mc bool) error {
	err := t.ensureConfigmap(ctx, namespace, clusterName, managementClusterName, mc, token)
	if err != nil {
		return microerror.Mask(err)
	}

	err = t.ensureApp(ctx, namespace, clusterName, managementClusterName, mc)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (t *TeleportApp) ensureConfigmap(ctx context.Context, namespace string, clusterName string, managementClusterName string, mc bool, token string) error {
	logger := t.logger.WithValues("cluster", clusterName)

	name := key.ConfigmapName(t.appName)

	dataTmpl := `roles: "%s"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`

	kubeClusterName := clusterName
	if mc == false {
		kubeClusterName = fmt.Sprintf("%s-%s", managementClusterName, clusterName)
	}

	data := map[string]string{
		"values": fmt.Sprintf(dataTmpl, "kube", token, t.teleportProxyAddr, kubeClusterName),
	}

	if t.teleportVersion != "" {
		data["values"] = fmt.Sprintf("%steleportVersionOverride: %q", data["values"], t.teleportVersion)
	}

	logger.Info("Checking if configmap exists")

	// look for existing configmap
	cm := corev1.ConfigMap{}
	err := t.ctrlClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &cm)
	if errors.IsNotFound(err) {
		logger.Info("Configmap doesn't exist")

		// CM not existing, create it
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

		logger.Info("Configmap created")
		return nil
	}

	logger.Info("Checking if configmap is up to date")

	// ensure cm is up-to-date
	if !reflect.DeepEqual(cm.Data, data) {
		logger.Info("Updating configmap")
		cm.Data = data
		err = t.ctrlClient.Update(ctx, &cm)
		if err != nil {
			return microerror.Mask(err)
		}

		logger.Info("Updated configmap")
		return nil
	}

	logger.Info("Configmap was up to date")

	return nil
}

func (t *TeleportApp) ensureApp(ctx context.Context, appNamespace string, clusterName string, managementClusterName string, mc bool) error {
	logger := t.logger.WithValues("cluster", clusterName)

	appSpecKubeConfig := appv1alpha1.AppSpecKubeConfig{
		InCluster: mc,
	}

	if mc == false {
		appSpecKubeConfig.Context = appv1alpha1.AppSpecKubeConfigContext{
			Name: fmt.Sprintf("%s-%s", managementClusterName, clusterName),
		}
		appSpecKubeConfig.Secret = appv1alpha1.AppSpecKubeConfigSecret{
			Name:      fmt.Sprintf("%s-kubeconfig", clusterName),
			Namespace: appNamespace,
		}
	}

	appSpec := appv1alpha1.AppSpec{
		Catalog:    t.appCatalog,
		KubeConfig: appSpecKubeConfig,
		Name:       t.appName,
		Namespace:  "kube-system",
		UserConfig: appv1alpha1.AppSpecUserConfig{
			ConfigMap: appv1alpha1.AppSpecUserConfigConfigMap{
				Name:      key.ConfigmapName(t.appName),
				Namespace: appNamespace,
			},
		},
		Version: t.appVersion,
	}

	logger.Info("Checking if app exists")

	desiredApp := appv1alpha1.App{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", clusterName, t.appName),
			Namespace: appNamespace,
			Labels: map[string]string{
				label.ManagedBy: project.Name(),
				label.Cluster:   clusterName,
			},
		},
		Spec: appSpec,
	}

	if mc == true {
		desiredApp.Labels[label.AppOperatorVersion] = "0.0.0"
	}

	// look for existing app
	app := appv1alpha1.App{}
	err := t.ctrlClient.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("%s-%s", clusterName, t.appName), Namespace: appNamespace}, &app)
	if errors.IsNotFound(err) {
		logger.Info("App doesn't exist")

		err = t.ctrlClient.Create(ctx, &desiredApp)
		if err != nil {
			return microerror.Mask(err)
		}

		return nil
	} else if err != nil {
		return microerror.Mask(err)
	}

	logger.Info("App created")

	logger.Info("Checking if app is up to date")

	// ensure app is up-to-date
	if !reflect.DeepEqual(app.Spec, appSpec) {
		logger.Info("App is outdated, deleting it")

		err = t.ctrlClient.Delete(ctx, &app)
		if err != nil {
			return microerror.Mask(err)
		}

		logger.Info("Deleted app")

		logger.Info("Creating app")

		err = t.ctrlClient.Create(ctx, &desiredApp)
		if err != nil {
			return microerror.Mask(err)
		}

		logger.Info("Created app")
		return nil
	}

	logger.Info("App was up to date")

	return nil
}
