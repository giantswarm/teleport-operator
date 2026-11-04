package teleport

import (
	"context"
	"testing"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/test"
)

const (
	testResourceName  = "test-resource"
	testNamespace     = "test-namespace"
	testConfigMapName = "test-configmap"
)

func Test_NewTeleportAppConfigManager_NoResource(t *testing.T) {
	fakeClient, err := test.NewFakeK8sClientFromObjects()
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, isNoOp := mgr.(*noOpTeleportAppConfigManager)
	if !isNoOp {
		t.Errorf("expected noOpTeleportAppConfigManager, got %T", mgr)
	}

	// No-op should succeed without panicking.
	log := ctrl.Log.WithName("test")
	if err := mgr.EnsureConfig(context.Background(), log); err != nil {
		t.Errorf("unexpected error from noOp EnsureConfig: %v", err)
	}
	if err := mgr.DeleteConfig(context.Background(), log); err != nil {
		t.Errorf("unexpected error from noOp DeleteConfig: %v", err)
	}
}

func Test_NewTeleportAppConfigManager_HelmReleaseExists(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, isHR := mgr.(*helmReleaseTeleportAppConfigManager)
	if !isHR {
		t.Errorf("expected helmReleaseTeleportAppConfigManager, got %T", mgr)
	}
}

func Test_NewTeleportAppConfigManager_AppCRExists(t *testing.T) {
	app := test.NewApp(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, isApp := mgr.(*appCRTeleportAppConfigManager)
	if !isApp {
		t.Errorf("expected appCRTeleportAppConfigManager, got %T", mgr)
	}
}

func Test_NewTeleportAppConfigManager_BothExist_HelmReleaseTakesPrecedence(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	app := test.NewApp(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(hr, app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, isHR := mgr.(*helmReleaseTeleportAppConfigManager)
	if !isHR {
		t.Errorf("expected helmReleaseTeleportAppConfigManager when both exist, got %T", mgr)
	}
}

// --- HelmRelease tests ---

// Flux owns spec.valuesFrom, so neither call may write the HelmRelease, whether
// or not the entry is there.
func Test_HelmRelease_EnsureAndDeleteConfig_NeverWrite(t *testing.T) {
	declared := map[string]interface{}{
		"kind":      "ConfigMap",
		"name":      testConfigMapName,
		"valuesKey": "values",
		"optional":  false,
	}
	cases := map[string][]interface{}{
		"entry missing":  nil,
		"entry declared": {declared},
	}
	for name, valuesFrom := range cases {
		t.Run(name, func(t *testing.T) {
			hr := test.NewHelmRelease(testResourceName, testNamespace)
			if valuesFrom != nil {
				hr.Object["spec"] = map[string]interface{}{"valuesFrom": valuesFrom}
			}
			fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
			if err != nil {
				t.Fatalf("failed to create fake client: %v", err)
			}
			before := getHelmRelease(t, fakeClient)

			mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			log := ctrl.Log.WithName("test")
			if err := mgr.EnsureConfig(context.Background(), log); err != nil {
				t.Fatalf("EnsureConfig returned error: %v", err)
			}
			if err := mgr.DeleteConfig(context.Background(), log); err != nil {
				t.Fatalf("DeleteConfig returned error: %v", err)
			}

			after := getHelmRelease(t, fakeClient)
			if after.GetResourceVersion() != before.GetResourceVersion() {
				t.Errorf("expected the HelmRelease to be untouched, resourceVersion %s -> %s",
					before.GetResourceVersion(), after.GetResourceVersion())
			}
		})
	}
}

func getHelmRelease(t *testing.T, c client.Client) *unstructured.Unstructured {
	t.Helper()
	hr := newHelmReleaseUnstructured()
	if err := c.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, hr); err != nil {
		t.Fatalf("failed to get HelmRelease: %v", err)
	}
	return hr
}

// --- App CR EnsureConfig tests ---

func Test_AppCR_EnsureConfig_AddsEntry(t *testing.T) {
	app := test.NewApp(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	log := ctrl.Log.WithName("test")
	if err := mgr.EnsureConfig(context.Background(), log); err != nil {
		t.Fatalf("EnsureConfig returned error: %v", err)
	}

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get App: %v", err)
	}

	if len(updated.Spec.ExtraConfigs) != 1 {
		t.Fatalf("expected 1 ExtraConfigs entry, got %d", len(updated.Spec.ExtraConfigs))
	}
	cfg := updated.Spec.ExtraConfigs[0]
	if cfg.Kind != "configMap" || cfg.Name != testConfigMapName || cfg.Namespace != testNamespace || cfg.Priority != 25 {
		t.Errorf("unexpected ExtraConfigs entry: %+v", cfg)
	}
}

func Test_AppCR_EnsureConfig_Idempotent(t *testing.T) {
	app := test.NewApp(testResourceName, testNamespace)
	app.Spec.ExtraConfigs = []appv1alpha1.AppExtraConfig{
		{Kind: "configMap", Name: testConfigMapName, Namespace: testNamespace, Priority: 25},
	}
	fakeClient, err := test.NewFakeK8sClientFromObjects(app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	log := ctrl.Log.WithName("test")
	if err := mgr.EnsureConfig(context.Background(), log); err != nil {
		t.Fatalf("EnsureConfig returned error: %v", err)
	}

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get App: %v", err)
	}

	if len(updated.Spec.ExtraConfigs) != 1 {
		t.Errorf("expected 1 ExtraConfigs entry (no duplicate), got %d", len(updated.Spec.ExtraConfigs))
	}
}

func Test_AppCR_DeleteConfig_RemovesEntry(t *testing.T) {
	app := test.NewApp(testResourceName, testNamespace)
	app.Spec.ExtraConfigs = []appv1alpha1.AppExtraConfig{
		{Kind: "configMap", Name: testConfigMapName, Namespace: testNamespace, Priority: 25},
	}
	fakeClient, err := test.NewFakeK8sClientFromObjects(app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	log := ctrl.Log.WithName("test")
	if err := mgr.DeleteConfig(context.Background(), log); err != nil {
		t.Fatalf("DeleteConfig returned error: %v", err)
	}

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get App: %v", err)
	}

	if len(updated.Spec.ExtraConfigs) != 0 {
		t.Errorf("expected empty ExtraConfigs after delete, got %d entries", len(updated.Spec.ExtraConfigs))
	}
}

func Test_AppCR_DeleteConfig_NoOp_WhenEntryAbsent(t *testing.T) {
	app := test.NewApp(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	mgr, err := NewTeleportAppConfigManager(context.Background(), fakeClient, testResourceName, testNamespace, testConfigMapName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	log := ctrl.Log.WithName("test")
	if err := mgr.DeleteConfig(context.Background(), log); err != nil {
		t.Fatalf("DeleteConfig returned error: %v", err)
	}

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get App: %v", err)
	}
	if len(updated.Spec.ExtraConfigs) != 0 {
		t.Errorf("expected ExtraConfigs unchanged (nil/empty), got %d entries", len(updated.Spec.ExtraConfigs))
	}
}

func Test_GetTeleportKubeAgentVersion_NoResource(t *testing.T) {
	fakeClient, err := test.NewFakeK8sClientFromObjects()
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	v, err := GetTeleportKubeAgentVersion(context.Background(), fakeClient, testResourceName, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "" {
		t.Errorf("expected empty version, got %q", v)
	}
}

func Test_GetTeleportKubeAgentVersion_AppCR(t *testing.T) {
	app := test.NewApp(testResourceName, testNamespace)
	app.Spec.Version = "0.11.0"

	fakeClient, err := test.NewFakeK8sClientFromObjects(app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	v, err := GetTeleportKubeAgentVersion(context.Background(), fakeClient, testResourceName, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "0.11.0" {
		t.Errorf("expected 0.11.0, got %q", v)
	}
}

func Test_GetTeleportKubeAgentVersion_HelmRelease_FromStatusHistory(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	if err := unstructured.SetNestedSlice(hr.Object, []interface{}{
		map[string]interface{}{"chartVersion": "0.11.0+abc"},
	}, "status", "history"); err != nil {
		t.Fatalf("set history: %v", err)
	}

	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	v, err := GetTeleportKubeAgentVersion(context.Background(), fakeClient, testResourceName, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "0.11.0+abc" {
		t.Errorf("expected 0.11.0+abc, got %q", v)
	}
}

func Test_GetTeleportKubeAgentVersion_HelmRelease_FromSpecChart(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	if err := unstructured.SetNestedField(hr.Object, "0.11.0", "spec", "chart", "spec", "version"); err != nil {
		t.Fatalf("set spec.chart.spec.version: %v", err)
	}

	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	v, err := GetTeleportKubeAgentVersion(context.Background(), fakeClient, testResourceName, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "0.11.0" {
		t.Errorf("expected 0.11.0, got %q", v)
	}
}

func Test_GetTeleportKubeAgentVersion_HelmReleaseTakesPrecedence(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	if err := unstructured.SetNestedSlice(hr.Object, []interface{}{
		map[string]interface{}{"chartVersion": "0.11.0-hr"},
	}, "status", "history"); err != nil {
		t.Fatalf("set history: %v", err)
	}

	app := test.NewApp(testResourceName, testNamespace)
	app.Spec.Version = "0.10.0-app"

	fakeClient, err := test.NewFakeK8sClientFromObjects(hr, app)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	v, err := GetTeleportKubeAgentVersion(context.Background(), fakeClient, testResourceName, testNamespace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "0.11.0-hr" {
		t.Errorf("expected HelmRelease to win, got %q", v)
	}
}
