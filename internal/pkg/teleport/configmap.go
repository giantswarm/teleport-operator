package teleport

import (
	"context"

	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Teleport) CreateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, registerName string, installNamespace string, token string) error {
	configMapName := key.GetConfigmapName(clusterName, t.SecretConfig.AppName)

	configMapData := map[string]string{
		"values": t.getConfigMapData(registerName, token),
	}

	cm := corev1.ConfigMap{}
	if err := ctrlClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: installNamespace}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			cm := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: installNamespace,
					Labels: map[string]string{
						label.ManagedBy: key.TeleportOperatorLabelValue,
					},
				},
				Data: configMapData,
			}

			if err = ctrlClient.Create(ctx, &cm); err != nil {
				return microerror.Mask(err)
			}

			log.Info("Created configmap", "configMapName", configMapName)
			return nil
		}

		return microerror.Mask(err)
	}

	return nil
}

func (t *Teleport) DeleteConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string) error {
	configMapName := key.GetConfigmapName(clusterName, t.SecretConfig.AppName)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: clusterNamespace,
		},
	}

	if err := ctrlClient.Delete(ctx, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}

	log.Info("Deleted configmap", "configMap", configMapName)
	return nil
}

func (t *Teleport) getConfigMapData(registerName string, token string) string {
	var (
		authToken               = token
		proxyAddr               = t.SecretConfig.ProxyAddr
		kubeClusterName         = registerName
		teleportVersionOverride = t.SecretConfig.TeleportVersion
	)

	return key.GetConfigmapDataFromTemplate(authToken, proxyAddr, kubeClusterName, teleportVersionOverride)
}
