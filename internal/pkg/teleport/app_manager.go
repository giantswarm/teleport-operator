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

// GetTeleportKubeAgentVersion returns the chart version of the deployed
// teleport-kube-agent for a cluster, or "" if no matching HelmRelease or
// App CR exists. HelmRelease takes precedence (same order as
// NewTeleportAppConfigManager). For HelmReleases we prefer
// status.history[0].chartVersion (Flux's actually-installed version,
// independent of how the chart reference was authored), falling back to
// spec.chart.spec.version for HelmReleases that haven't reconciled yet.
// Callers treat the empty string as "unknown / pre-0.11.0".
func GetTeleportKubeAgentVersion(
	ctx context.Context,
	ctrlClient client.Client,
	resourceName, namespace string,
) (string, error) {
	hr := newHelmReleaseUnstructured()
	err := ctrlClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: namespace}, hr)
	if err == nil {
		if v := helmReleaseInstalledChartVersion(hr); v != "" {
			return v, nil
		}
		if v := helmReleaseSpecChartVersion(hr); v != "" {
			return v, nil
		}
		return "", nil
	}
	if !apierrors.IsNotFound(err) {
		return "", microerror.Mask(err)
	}

	app := &v1alpha1.App{}
	err = ctrlClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: namespace}, app)
	if err == nil {
		return app.Spec.Version, nil
	}
	if !apierrors.IsNotFound(err) {
		return "", microerror.Mask(err)
	}

	return "", nil
}

func helmReleaseInstalledChartVersion(hr *unstructured.Unstructured) string {
	status, ok := hr.Object["status"].(map[string]interface{})
	if !ok {
		return ""
	}
	history, ok := status["history"].([]interface{})
	if !ok || len(history) == 0 {
		return ""
	}
	entry, ok := history[0].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := entry["chartVersion"].(string)
	return v
}

func helmReleaseSpecChartVersion(hr *unstructured.Unstructured) string {
	spec, ok := hr.Object["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	chart, ok := spec["chart"].(map[string]interface{})
	if !ok {
		return ""
	}
	chartSpec, ok := chart["spec"].(map[string]interface{})
	if !ok {
		return ""
	}
	v, _ := chartSpec["version"].(string)
	return v
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
		if sameValuesReference(existing, ref) {
			return refs
		}
	}
	return append(refs, ref)
}

// removeValuesReference drops only an entry we wrote ourselves, so it compares
// exactly. The loosened comparison used when appending must NOT be reused here:
// an entry the parent cluster chart rendered is owned by that chart, and it is
// the chart's job to stop rendering it — deleting it on our behalf would fight
// the owner over a field we deliberately no longer manage.
func removeValuesReference(refs []interface{}, ref map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(refs))
	for _, existing := range refs {
		if !reflect.DeepEqual(existing, ref) {
			result = append(result, existing)
		}
	}
	return result
}

// sameValuesReference reports whether an existing `valuesFrom` entry already
// points at the same values as ref, ignoring `optional`.
//
// The cluster chart declares this operator's ConfigMap in the HelmRelease's
// valuesFrom itself and renders every entry with an explicit
// `optional: false` (see the cluster.app.sortedValuesFrom helper). A plain
// deep-equal against our own three-key entry therefore never matches the
// chart-declared one, and we append a second reference to the very same
// ConfigMap. `optional` only decides whether a missing source is an error, not
// which values get loaded, so it must not take part in identity.
//
// Any other extra field — `targetPath` above all, which places the values at a
// subpath and so means something genuinely different — still forces a
// non-match, keeping the comparison conservative.
//
// This governs append identity only, and it prevents new duplicates rather than
// removing existing ones: on a HelmRelease that already carries both the
// chart-declared entry and a duplicate we appended earlier, the chart-declared
// entry now matches first and we leave the list alone. That is deliberate.
// Rewriting the list to prune the duplicate would mean another write to a field
// owned by the chart, re-taking ownership of an atomic list — the very thing
// this change exists to stop. The stale duplicate is harmless: it references the
// same ConfigMap and key, so merging it twice is a no-op, and it disappears the
// next time the owner re-applies the field.
func sameValuesReference(existing interface{}, ref map[string]interface{}) bool {
	entry, ok := existing.(map[string]interface{})
	if !ok {
		return false
	}
	if len(entry) > 0 {
		trimmed := make(map[string]interface{}, len(entry))
		for k, v := range entry {
			if k == "optional" {
				continue
			}
			trimmed[k] = v
		}
		entry = trimmed
	}
	return reflect.DeepEqual(entry, ref)
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
