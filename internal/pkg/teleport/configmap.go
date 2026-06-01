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
	valuesYaml, err := parseConfigMapValues(configMap)
	if err != nil {
		return "", err
	}

	token, ok := valuesYaml["authToken"].(string)
	if !ok {
		return "", microerror.Mask(fmt.Errorf("malformed ConfigMap: key `authToken` not found"))
	}

	return token, nil
}

// GetTeleportVersionFromConfigMap returns the teleportVersionOverride
// currently stored in the ConfigMap, or an empty string if not set.
func (t *Teleport) GetTeleportVersionFromConfigMap(configMap *corev1.ConfigMap) (string, error) {
	valuesYaml, err := parseConfigMapValues(configMap)
	if err != nil {
		return "", err
	}

	version, _ := valuesYaml["teleportVersionOverride"].(string)
	return version, nil
}

// IsConfigMapLayoutUpToDate reports whether the ConfigMap's top-level shape
// matches what the cluster's deployed teleport-kube-agent chart version
// expects:
//
//   - tkaVersion >= v0.11.0: nested-only — the `teleport-kube-agent:` block
//     must exist and the flat block must be absent (root has no authToken).
//   - tkaVersion < v0.11.0 or unknown: dual — both the nested block AND
//     the flat root keys must be present.
func (t *Teleport) IsConfigMapLayoutUpToDate(configMap *corev1.ConfigMap, tkaVersion string) (bool, error) {
	valuesBytes, ok := configMap.Data["values"]
	if !ok {
		return false, microerror.Mask(fmt.Errorf("malformed ConfigMap: key `values` not found"))
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesBytes), &root); err != nil {
		return false, microerror.Mask(fmt.Errorf("failed to parse YAML: %w", err))
	}

	_, hasNested := root[key.TeleportKubeAgentValuesKey].(map[string]interface{})
	_, hasFlat := root["authToken"].(string)

	if key.UsesNestedKubeAgentValues(tkaVersion) {
		return hasNested && !hasFlat, nil
	}
	return hasNested && hasFlat, nil
}

func parseConfigMapValues(configMap *corev1.ConfigMap) (map[string]interface{}, error) {
	valuesBytes, ok := configMap.Data["values"]
	if !ok {
		return nil, microerror.Mask(fmt.Errorf("malformed ConfigMap: key `values` not found"))
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(valuesBytes), &root); err != nil {
		return nil, microerror.Mask(fmt.Errorf("failed to parse YAML: %w", err))
	}

	if nested, ok := root[key.TeleportKubeAgentValuesKey].(map[string]interface{}); ok {
		return nested, nil
	}
	return root, nil
}

func (t *Teleport) CreateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string, registerName string, token string, roles []string, tkaVersion string) error {
	configMapName := key.GetConfigmapName(clusterName, t.Config.AppName)

	configMapData := map[string]string{
		"values": t.getConfigMapData(registerName, token, roles, tkaVersion),
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

// UpdateConfigMap rewrites the ConfigMap's `values` from the template so the
// produced YAML matches what GetConfigmapDataFromTemplate would emit for the
// given tkaVersion. This means the controller can do a single string compare
// to detect drift, and a tkaVersion crossing 0.11.0 actually drops the flat
// block from the stored ConfigMap.
func (t *Teleport) UpdateConfigMap(ctx context.Context, log logr.Logger, ctrlClient client.Client, configMap *corev1.ConfigMap, token string, roles []string, tkaVersion string) error {
	registerName, err := registerNameFromConfigMap(configMap)
	if err != nil {
		return err
	}

	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}
	configMap.Data["values"] = t.getConfigMapData(registerName, token, roles, tkaVersion)
	if err := ctrlClient.Update(ctx, configMap); err != nil {
		return microerror.Mask(fmt.Errorf("failed to update ConfigMap: %w", err))
	}
	log.Info("Updated config map with new teleport kube join token", "configMap", configMap.GetName())
	return nil
}

// registerNameFromConfigMap extracts the kubeClusterName from the stored
// values — it's the register name and doesn't change between reconciles.
func registerNameFromConfigMap(configMap *corev1.ConfigMap) (string, error) {
	values, err := parseConfigMapValues(configMap)
	if err != nil {
		return "", err
	}
	name, ok := values["kubeClusterName"].(string)
	if !ok || name == "" {
		return "", microerror.Mask(fmt.Errorf("malformed ConfigMap: key `kubeClusterName` not found"))
	}
	return name, nil
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

// RenderConfigMapValues returns the deterministic YAML the operator wants
// the cluster's teleport-kube-agent values ConfigMap to contain. The
// controller uses it both for the initial write and for byte-compare
// drift detection on subsequent reconciles.
func (t *Teleport) RenderConfigMapValues(registerName, token string, roles []string, tkaVersion string) string {
	return t.getConfigMapData(registerName, token, roles, tkaVersion)
}

func (t *Teleport) getConfigMapData(registerName, token string, roles []string, tkaVersion string) string {
	return key.GetConfigmapDataFromTemplate(token, t.Config.ProxyAddr, registerName, t.Config.TeleportVersion, roles, tkaVersion)
}

func (t *Teleport) getTbotConfigMapData(registerName string, clusterName string) string {
	return key.GetTbotConfigmapDataFromTemplate(registerName, clusterName)
}
