package teleport

import (
	"context"
	"reflect"

	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var helmReleaseGVK = schema.GroupVersionKind{
	Group:   "helm.toolkit.fluxcd.io",
	Version: "v2",
	Kind:    "HelmRelease",
}

func newHelmReleaseUnstructured() *unstructured.Unstructured {
	hr := &unstructured.Unstructured{}
	hr.SetGroupVersionKind(helmReleaseGVK)
	return hr
}

// TeleportAppConfigManager abstracts injecting a ConfigMap reference into either a
// Giant Swarm App CR (via spec.extraConfigs) or a Flux HelmRelease (via
// spec.valuesFrom).
type TeleportAppConfigManager interface {
	EnsureConfig(ctx context.Context, log logr.Logger) error
	DeleteConfig(ctx context.Context, log logr.Logger) error
}

// NewTeleportAppConfigManager detects at call time whether the named resource is a
// Flux HelmRelease or a Giant Swarm App CR and returns the appropriate manager.
// HelmRelease takes precedence when both exist. Never returns nil — returns a
// noOpTeleportAppConfigManager when neither resource is found.
func NewTeleportAppConfigManager(
	ctx context.Context,
	ctrlClient client.Client,
	resourceName string,
	namespace string,
	configMapName string,
) (TeleportAppConfigManager, error) {
	hr := newHelmReleaseUnstructured()
	err := ctrlClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: namespace}, hr)
	if err == nil {
		return &helmReleaseTeleportAppConfigManager{
			client:        ctrlClient,
			resourceName:  resourceName,
			namespace:     namespace,
			configMapName: configMapName,
		}, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, microerror.Mask(err)
	}

	app := &v1alpha1.App{}
	err = ctrlClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: namespace}, app)
	if err == nil {
		return &appCRTeleportAppConfigManager{
			client:        ctrlClient,
			resourceName:  resourceName,
			namespace:     namespace,
			configMapName: configMapName,
		}, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, microerror.Mask(err)
	}

	return &noOpTeleportAppConfigManager{
		resourceName: resourceName,
		namespace:    namespace,
	}, nil
}

// --- HelmRelease implementation ---

type helmReleaseTeleportAppConfigManager struct {
	client        client.Client
	resourceName  string
	namespace     string
	configMapName string
}

func (m *helmReleaseTeleportAppConfigManager) EnsureConfig(ctx context.Context, log logr.Logger) error {
	hr := newHelmReleaseUnstructured()
	if err := m.client.Get(ctx, client.ObjectKey{Name: m.resourceName, Namespace: m.namespace}, hr); err != nil {
		return microerror.Mask(err)
	}

	desired := map[string]interface{}{
		"kind":      "ConfigMap",
		"name":      m.configMapName,
		"valuesKey": "values",
	}

	before := getValuesFrom(hr)
	updated := appendValuesReference(before, desired)
	if reflect.DeepEqual(before, updated) {
		return nil
	}

	setValuesFrom(hr, updated)

	log.Info("Updating HelmRelease ValuesFrom", "helmrelease", m.resourceName, "configMap", m.configMapName)
	if err := m.client.Update(ctx, hr); err != nil {
		if apierrors.IsConflict(err) {
			log.Error(err, "Conflict updating HelmRelease, will requeue", "helmrelease", m.resourceName)
		}
		return microerror.Mask(err)
	}
	return nil
}

func (m *helmReleaseTeleportAppConfigManager) DeleteConfig(ctx context.Context, log logr.Logger) error {
	hr := newHelmReleaseUnstructured()
	if err := m.client.Get(ctx, client.ObjectKey{Name: m.resourceName, Namespace: m.namespace}, hr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}

	desired := map[string]interface{}{
		"kind":      "ConfigMap",
		"name":      m.configMapName,
		"valuesKey": "values",
	}

	before := getValuesFrom(hr)
	updated := removeValuesReference(before, desired)
	if reflect.DeepEqual(before, updated) {
		return nil
	}

	setValuesFrom(hr, updated)

	log.Info("Removing HelmRelease ValuesFrom entry", "helmrelease", m.resourceName, "configMap", m.configMapName)
	if err := m.client.Update(ctx, hr); err != nil {
		if apierrors.IsConflict(err) {
			log.Error(err, "Conflict updating HelmRelease, will requeue", "helmrelease", m.resourceName)
		}
		return microerror.Mask(err)
	}
	return nil
}

func getValuesFrom(hr *unstructured.Unstructured) []interface{} {
	spec, ok := hr.Object["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	valuesFrom, ok := spec["valuesFrom"].([]interface{})
	if !ok {
		return nil
	}
	return valuesFrom
}

func setValuesFrom(hr *unstructured.Unstructured, valuesFrom []interface{}) {
	spec, ok := hr.Object["spec"].(map[string]interface{})
	if !ok {
		spec = map[string]interface{}{}
		hr.Object["spec"] = spec
	}
	spec["valuesFrom"] = valuesFrom
}

func appendValuesReference(refs []interface{}, ref map[string]interface{}) []interface{} {
	for _, existing := range refs {
		if reflect.DeepEqual(existing, ref) {
			return refs
		}
	}
	return append(refs, ref)
}

func removeValuesReference(refs []interface{}, ref map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(refs))
	for _, existing := range refs {
		if !reflect.DeepEqual(existing, ref) {
			result = append(result, existing)
		}
	}
	return result
}

// --- App CR implementation ---

type appCRTeleportAppConfigManager struct {
	client        client.Client
	resourceName  string
	namespace     string
	configMapName string
}

func (m *appCRTeleportAppConfigManager) EnsureConfig(ctx context.Context, log logr.Logger) error {
	app := &v1alpha1.App{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: m.resourceName, Namespace: m.namespace}, app); err != nil {
		return microerror.Mask(err)
	}

	desired := v1alpha1.AppExtraConfig{
		Kind:      "configMap",
		Name:      m.configMapName,
		Namespace: m.namespace,
		Priority:  25,
	}

	before := app.Spec.ExtraConfigs
	app.Spec.ExtraConfigs = appendExtraConfig(before, desired)
	if reflect.DeepEqual(before, app.Spec.ExtraConfigs) {
		return nil
	}

	log.Info("Updating App ExtraConfigs", "app", m.resourceName, "configMap", m.configMapName)
	if err := m.client.Update(ctx, app); err != nil {
		if apierrors.IsConflict(err) {
			log.Error(err, "Conflict updating App, will requeue", "app", m.resourceName)
		}
		return microerror.Mask(err)
	}
	return nil
}

func (m *appCRTeleportAppConfigManager) DeleteConfig(ctx context.Context, log logr.Logger) error {
	app := &v1alpha1.App{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: m.resourceName, Namespace: m.namespace}, app); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}

	if app.Spec.ExtraConfigs == nil {
		return nil
	}

	desired := v1alpha1.AppExtraConfig{
		Kind:      "configMap",
		Name:      m.configMapName,
		Namespace: m.namespace,
		Priority:  25,
	}

	before := app.Spec.ExtraConfigs
	app.Spec.ExtraConfigs = removeExtraConfig(before, desired)
	if reflect.DeepEqual(before, app.Spec.ExtraConfigs) {
		return nil
	}

	log.Info("Removing App ExtraConfigs entry", "app", m.resourceName, "configMap", m.configMapName)
	if err := m.client.Update(ctx, app); err != nil {
		if apierrors.IsConflict(err) {
			log.Error(err, "Conflict updating App, will requeue", "app", m.resourceName)
		}
		return microerror.Mask(err)
	}
	return nil
}

func appendExtraConfig(configs []v1alpha1.AppExtraConfig, config v1alpha1.AppExtraConfig) []v1alpha1.AppExtraConfig {
	for _, existing := range configs {
		if reflect.DeepEqual(existing, config) {
			return configs
		}
	}
	return append(configs, config)
}

func removeExtraConfig(configs []v1alpha1.AppExtraConfig, config v1alpha1.AppExtraConfig) []v1alpha1.AppExtraConfig {
	result := make([]v1alpha1.AppExtraConfig, 0, len(configs))
	for _, existing := range configs {
		if !reflect.DeepEqual(existing, config) {
			result = append(result, existing)
		}
	}
	return result
}

// --- No-op implementation ---

type noOpTeleportAppConfigManager struct {
	resourceName string
	namespace    string
}

func (n *noOpTeleportAppConfigManager) EnsureConfig(ctx context.Context, log logr.Logger) error {
	log.Info("No HelmRelease or App CR found, skipping config injection",
		"resource", n.resourceName, "namespace", n.namespace)
	return nil
}

func (n *noOpTeleportAppConfigManager) DeleteConfig(ctx context.Context, log logr.Logger) error {
	log.Info("No HelmRelease or App CR found, skipping config deletion",
		"resource", n.resourceName, "namespace", n.namespace)
	return nil
}
