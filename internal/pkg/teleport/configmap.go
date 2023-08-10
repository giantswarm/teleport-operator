package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"

	"gopkg.in/yaml.v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *Teleport) GetConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string) (*corev1.ConfigMap, error) {
	var (
		configMapName = key.GetConfigmapName(clusterName, t.SecretConfig.AppName)
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

func (t *Teleport) GetTokenFromConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (string, error) {
	valuesBytes, ok := configMap.Data["values"]
	if !ok {
		return "", microerror.Mask(fmt.Errorf("malformed ConfigMap: key `values` not found"))
	}

	var parsedContent map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesBytes), &parsedContent); err != nil {
		return "", microerror.Mask(fmt.Errorf("failed to parse YAML: %w", err))
	}

	authToken, ok := parsedContent["authToken"].(string)
	if !ok {
		return "", microerror.Mask(fmt.Errorf("malformed ConfigMap: key `authToken` not found"))
	}

	return authToken, nil
}

func (t *Teleport) CreateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string, registerName string, token string) error {
	configMapName := key.GetConfigmapName(clusterName, t.SecretConfig.AppName)

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

func (t *Teleport) UpdateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, configMap *corev1.ConfigMap, token string) error {
	yamlContent, exists := configMap.Data["values"]
	if !exists {
		return fmt.Errorf("key 'values' not found in the ConfigMap")
	}

	var parsedContent map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &parsedContent); err != nil {
		return fmt.Errorf("failed to parse YAML: %v", err)
	}

	// Modify the authToken value
	parsedContent["authToken"] = token

	updatedYamlContent, err := yaml.Marshal(parsedContent)
	if err != nil {
		return fmt.Errorf("failed to marshal updated content into YAML: %v", err)
	}

	// Update the ConfigMap's data with the modified value
	configMap.Data["values"] = string(updatedYamlContent)
	if err := ctrlClient.Update(ctx, configMap); err != nil {
		return microerror.Mask(fmt.Errorf("failed to update ConfigMap: %w", err))
	}
	log.Info("Updated config map with new teleport kube join token", "configMap", configMap.GetName())
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
