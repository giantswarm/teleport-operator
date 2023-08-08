package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Teleport) CreateConfigMap(ctx context.Context, config *TeleportConfig, token string) error {
	configMapName := key.GetConfigmapName(config.Cluster.Name, t.SecretConfig.AppName)

	configMapData := map[string]string{
		"values": t.getConfigMapData(config, token),
	}

	cm := corev1.ConfigMap{}
	if err := config.CtrlClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: config.InstallNamespace}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			cm := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: config.InstallNamespace,
					Labels: map[string]string{
						label.ManagedBy: key.TeleportOperatorLabelValue,
					},
				},
				Data: configMapData,
			}

			if err = config.CtrlClient.Create(ctx, &cm); err != nil {
				return microerror.Mask(err)
			}

			config.Log.Info("Created configmap", "configMapName", configMapName)
			return nil
		}

		return microerror.Mask(err)
	}

	return nil
}

func (t *Teleport) DeleteConfigMap(ctx context.Context, config *TeleportConfig) error {
	configMapName := key.GetConfigmapName(config.Cluster.Name, t.SecretConfig.AppName)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: config.Cluster.Namespace,
		},
	}

	if err := config.CtrlClient.Delete(ctx, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}

	config.Log.Info("Deleted configmap", "configMap", configMapName)
	return nil
}

func (t *Teleport) getConfigMapData(config *TeleportConfig, token string) string {
	var (
		authToken               = token
		proxyAddr               = t.SecretConfig.ProxyAddr
		kubeClusterName         = config.RegisterName
		teleportVersionOverride = t.SecretConfig.TeleportVersion
	)

	dataTpl := `roles: "kube"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
apps: []
`

	if t.SecretConfig.TeleportVersion != "" {
		dataTpl = fmt.Sprintf("%steleportVersionOverride: %q", dataTpl, teleportVersionOverride)
	}

	return fmt.Sprintf(dataTpl, authToken, proxyAddr, kubeClusterName)
}
