package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"

	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Teleport) GetConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string) (*corev1.ConfigMap, error) {
	var (
		configMapName = key.GetConfigmapName(clusterName, t.Config.AppName)
		configMap     = &corev1.ConfigMap{}
	)

	if err := ctrlClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: clusterNamespace}, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, microerror.Mask(fmt.Errorf("failed to get ConfigMap: %w", err))
	}

	return configMap, nil
}

func (t *Teleport) GetTbotConfigMap(ctx context.Context, ctrlClient client.Client, clusterName string) (*corev1.ConfigMap, error) {
	var (
		configMapName = key.GetTbotConfigmapName(clusterName)
		configMap     = &corev1.ConfigMap{}
	)

	key := client.ObjectKey{Name: configMapName, Namespace: key.TeleportBotNamespace}
	if err := ctrlClient.Get(ctx, key, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, microerror.Mask(fmt.Errorf("bot: Failed to get configmap: %w", err))
	}

	return configMap, nil
}

func (t *Teleport) GetTokenFromConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (string, error) {
	valuesBytes, ok := configMap.Data["values"]
	if !ok {
		return "", microerror.Mask(fmt.Errorf("malformed ConfigMap: key `values` not found"))
	}

	var valuesYaml map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesBytes), &valuesYaml); err != nil {
		return "", microerror.Mask(fmt.Errorf("failed to parse YAML: %w", err))
	}

	token, ok := valuesYaml["authToken"].(string)
	if !ok {
		return "", microerror.Mask(fmt.Errorf("malformed ConfigMap: key `authToken` not found"))
	}

	return token, nil
}

func (t *Teleport) CreateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string, registerName string, token string) error {
	configMapName := key.GetConfigmapName(clusterName, t.Config.AppName)

	configMapData := map[string]string{
		"values": t.getConfigMapData(registerName, token),
	}

	cm := corev1.ConfigMap{}
	if err := ctrlClient.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: clusterNamespace}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			cm := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: clusterNamespace,
				},
				Data: configMapData,
			}

			if err = ctrlClient.Create(ctx, &cm); err != nil {
				return microerror.Mask(err)
			}

			log.Info("Created config map with new teleport kube join token", "configMapName", configMapName)
			return nil
		}

		return microerror.Mask(err)
	}

	return nil
}

func (t *Teleport) CreateTbotConfigMap(ctx context.Context, ctrlClient client.Client, clusterName string, registerName string) (*corev1.ConfigMap, error) {
	configMapName := key.GetTbotConfigmapName(clusterName)
	data := map[string]string{
		"values": t.getTbotConfigMapData(registerName, clusterName),
	}
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: key.TeleportBotNamespace,
		},
		Data: data,
	}

	if err := ctrlClient.Create(ctx, &cm); err != nil {
		return nil, microerror.Mask(err)
	}

	return &cm, nil
}

func (t *Teleport) EnsureTbotConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, namespace string, registerName string) error {
	cm, err := t.GetTbotConfigMap(ctx, ctrlClient, clusterName)
	if err != nil {
		return microerror.Mask(err)
	}

	if cm == nil {
		cm, err = t.CreateTbotConfigMap(ctx, ctrlClient, clusterName, registerName)
		if err != nil {
			return microerror.Mask(err)
		}
		log.Info("tbot: Created configmap", "configmap", cm)
	}

	return nil
}

func (t *Teleport) UpdateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, configMap *corev1.ConfigMap, token string) error {
	valuesBytes, ok := configMap.Data["values"]
	if !ok {
		return microerror.Mask(fmt.Errorf("malformed ConfigMap: key `values` not found"))
	}

	var valuesYaml map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesBytes), &valuesYaml); err != nil {
		return microerror.Mask(fmt.Errorf("failed to parse YAML: %w", err))
	}

	// Modify the authToken value
	valuesYaml["authToken"] = token

	updatedValuesYaml, err := yaml.Marshal(valuesYaml)
	if err != nil {
		return fmt.Errorf("failed to marshal updated content into YAML: %w", err)
	}

	// Update the ConfigMap's data with the modified value
	configMap.Data["values"] = string(updatedValuesYaml)
	if err := ctrlClient.Update(ctx, configMap); err != nil {
		return microerror.Mask(fmt.Errorf("failed to update ConfigMap: %w", err))
	}
	log.Info("Updated config map with new teleport kube join token", "configMap", configMap.GetName())
	return nil
}

func (t *Teleport) DeleteConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string) error {
	configMapName := key.GetConfigmapName(clusterName, t.Config.AppName)
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

func (t *Teleport) DeleteTbotConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, namespace string) error {
	configMapName := key.GetTbotConfigmapName(clusterName)
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	if err := ctrlClient.Delete(ctx, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}

	log.Info("tbot: Deleted configmap", "configMap", configMapName)
	return nil
}

func (t *Teleport) getConfigMapData(registerName string, token string) string {
	var (
		authToken               = token
		proxyAddr               = t.Config.ProxyAddr
		kubeClusterName         = registerName
		teleportVersionOverride = t.Config.TeleportVersion
	)

	return key.GetConfigmapDataFromTemplate(authToken, proxyAddr, kubeClusterName, teleportVersionOverride)
}

func (t *Teleport) getTbotConfigMapData(registerName string, clusterName string) string {
	return key.GetTbotConfigmapDataFromTemplate(registerName, clusterName)
}
