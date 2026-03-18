package teleport

import (
	"context"
	"reflect"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TeleportAppManager abstracts injecting a ConfigMap reference into either a
// Giant Swarm App CR (via spec.extraConfigs) or a Flux HelmRelease (via
// spec.valuesFrom). Implementations are constructed per-cluster per-reconcile
// by NewTeleportAppManager.
type TeleportAppManager interface {
	EnsureConfig(ctx context.Context, log logr.Logger) error
	DeleteConfig(ctx context.Context, log logr.Logger) error
}

// NewTeleportAppManager detects at call time whether the named resource is a
// Flux HelmRelease or a Giant Swarm App CR and returns the appropriate manager.
// HelmRelease takes precedence when both exist. Never returns nil — returns a
// noOpTeleportAppManager when neither resource is found.
func NewTeleportAppManager(
	ctx context.Context,
	ctrlClient client.Client,
	resourceName string,
	namespace string,
	configMapName string,
) (TeleportAppManager, error) {
	hr := &helmv2.HelmRelease{}
	err := ctrlClient.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: namespace}, hr)
	if err == nil {
		return &helmReleaseTeleportAppManager{
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
		return &appCRTeleportAppManager{
			client:        ctrlClient,
			resourceName:  resourceName,
			namespace:     namespace,
			configMapName: configMapName,
		}, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, microerror.Mask(err)
	}

	return &noOpTeleportAppManager{
		resourceName: resourceName,
		namespace:    namespace,
	}, nil
}

// --- HelmRelease implementation ---

type helmReleaseTeleportAppManager struct {
	client        client.Client
	resourceName  string
	namespace     string
	configMapName string
}

func (m *helmReleaseTeleportAppManager) EnsureConfig(ctx context.Context, log logr.Logger) error {
	hr := &helmv2.HelmRelease{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: m.resourceName, Namespace: m.namespace}, hr); err != nil {
		return microerror.Mask(err)
	}

	desired := helmv2.ValuesReference{
		Kind:      "ConfigMap",
		Name:      m.configMapName,
		ValuesKey: "values",
	}

	before := hr.Spec.ValuesFrom
	hr.Spec.ValuesFrom = appendValuesReference(before, desired)
	if reflect.DeepEqual(before, hr.Spec.ValuesFrom) {
		return nil
	}

	log.Info("Updating HelmRelease ValuesFrom", "helmrelease", m.resourceName, "configMap", m.configMapName)
	if err := m.client.Update(ctx, hr); err != nil {
		if apierrors.IsConflict(err) {
			log.Error(err, "Conflict updating HelmRelease, will requeue", "helmrelease", m.resourceName)
		}
		return microerror.Mask(err)
	}
	return nil
}

func (m *helmReleaseTeleportAppManager) DeleteConfig(ctx context.Context, log logr.Logger) error {
	hr := &helmv2.HelmRelease{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: m.resourceName, Namespace: m.namespace}, hr); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}

	desired := helmv2.ValuesReference{
		Kind:      "ConfigMap",
		Name:      m.configMapName,
		ValuesKey: "values",
	}

	before := hr.Spec.ValuesFrom
	hr.Spec.ValuesFrom = removeValuesReference(before, desired)
	if reflect.DeepEqual(before, hr.Spec.ValuesFrom) {
		return nil
	}

	log.Info("Removing HelmRelease ValuesFrom entry", "helmrelease", m.resourceName, "configMap", m.configMapName)
	if err := m.client.Update(ctx, hr); err != nil {
		if apierrors.IsConflict(err) {
			log.Error(err, "Conflict updating HelmRelease, will requeue", "helmrelease", m.resourceName)
		}
		return microerror.Mask(err)
	}
	return nil
}

func appendValuesReference(refs []helmv2.ValuesReference, ref helmv2.ValuesReference) []helmv2.ValuesReference {
	for _, existing := range refs {
		if reflect.DeepEqual(existing, ref) {
			return refs
		}
	}
	return append(refs, ref)
}

func removeValuesReference(refs []helmv2.ValuesReference, ref helmv2.ValuesReference) []helmv2.ValuesReference {
	result := make([]helmv2.ValuesReference, 0, len(refs))
	for _, existing := range refs {
		if !reflect.DeepEqual(existing, ref) {
			result = append(result, existing)
		}
	}
	return result
}

// --- App CR implementation ---

type appCRTeleportAppManager struct {
	client        client.Client
	resourceName  string
	namespace     string
	configMapName string
}

func (m *appCRTeleportAppManager) EnsureConfig(ctx context.Context, log logr.Logger) error {
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

func (m *appCRTeleportAppManager) DeleteConfig(ctx context.Context, log logr.Logger) error {
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

type noOpTeleportAppManager struct {
	resourceName string
	namespace    string
}

func (n *noOpTeleportAppManager) EnsureConfig(ctx context.Context, log logr.Logger) error {
	log.Info("No HelmRelease or App CR found, skipping config injection",
		"resource", n.resourceName, "namespace", n.namespace)
	return nil
}

func (n *noOpTeleportAppManager) DeleteConfig(ctx context.Context, log logr.Logger) error {
	log.Info("No HelmRelease or App CR found, skipping config deletion",
		"resource", n.resourceName, "namespace", n.namespace)
	return nil
}
