package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/k8smetadata/pkg/label"
	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Teleport) EnsureConfigmap(ctx context.Context, config *InstallAppConfig) error {
	logger := t.Logger.WithValues("cluster", config.ClusterName)
	configMapName := key.GetConfigmapName(config.ClusterName, t.Secret.AppName)

	data := map[string]string{
		"values": t.getConfigmapValues(config),
	}

	if t.Secret.TeleportVersion != "" {
		data["values"] = fmt.Sprintf("%steleportVersionOverride: %q", data["values"], t.Secret.TeleportVersion)
	}

	cm := corev1.ConfigMap{}
	err := t.CtrlClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: config.InstallNamespace}, &cm)
	if errors.IsNotFound(err) {
		logger.Info("Configmap does not exist.")
		cm := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: config.InstallNamespace,
				Labels: map[string]string{
					label.ManagedBy: key.TeleportOperatorLabelValue,
				},
			},
			Data: data,
		}

		err = t.CtrlClient.Create(ctx, &cm)
		if err != nil {
			return microerror.Mask(err)
		}

		logger.Info("Configmap created.")
		return nil
	}

	logger.Info("Configmap exists.")
	return nil
}

func (t *Teleport) DeleteConfigMap(ctx context.Context, cluster *capi.Cluster) error {
	t.Logger.Info("Deleting config map...")
	configMapName := key.GetConfigmapName(cluster.Name, t.Secret.AppName)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: cluster.Namespace,
		},
	}
	if err := t.CtrlClient.Delete(ctx, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			t.Logger.Info("ConfigMap does not exist.")
			return nil
		}
		return microerror.Mask(err)
	}
	t.Logger.Info("ConfigMap deleted.")
	return nil
}

func (t *Teleport) getConfigmapValues(config *InstallAppConfig) string {
	dateTpl := `roles: "kube"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`
	return fmt.Sprintf(dateTpl, config.JoinToken, t.Secret.ProxyAddr, config.RegisterName)
}
