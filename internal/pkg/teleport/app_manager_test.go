package teleport

import (
	"context"
	"testing"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
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

// --- HelmRelease EnsureConfig tests ---

func Test_HelmRelease_EnsureConfig_AddsEntry(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
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

	updated := &helmv2.HelmRelease{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get HelmRelease: %v", err)
	}

	if len(updated.Spec.ValuesFrom) != 1 {
		t.Fatalf("expected 1 ValuesFrom entry, got %d", len(updated.Spec.ValuesFrom))
	}
	ref := updated.Spec.ValuesFrom[0]
	if ref.Kind != "ConfigMap" || ref.Name != testConfigMapName || ref.ValuesKey != "values" {
		t.Errorf("unexpected ValuesFrom entry: %+v", ref)
	}
}

func Test_HelmRelease_EnsureConfig_Idempotent(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	hr.Spec.ValuesFrom = []helmv2.ValuesReference{
		{Kind: "ConfigMap", Name: testConfigMapName, ValuesKey: "values"},
	}
	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
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

	updated := &helmv2.HelmRelease{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get HelmRelease: %v", err)
	}

	if len(updated.Spec.ValuesFrom) != 1 {
		t.Errorf("expected 1 ValuesFrom entry (no duplicate), got %d", len(updated.Spec.ValuesFrom))
	}
}

func Test_HelmRelease_DeleteConfig_RemovesEntry(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	hr.Spec.ValuesFrom = []helmv2.ValuesReference{
		{Kind: "ConfigMap", Name: testConfigMapName, ValuesKey: "values"},
	}
	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
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

	updated := &helmv2.HelmRelease{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get HelmRelease: %v", err)
	}

	if len(updated.Spec.ValuesFrom) != 0 {
		t.Errorf("expected empty ValuesFrom after delete, got %d entries", len(updated.Spec.ValuesFrom))
	}
}

func Test_HelmRelease_DeleteConfig_NoOp_WhenEntryAbsent(t *testing.T) {
	hr := test.NewHelmRelease(testResourceName, testNamespace)
	fakeClient, err := test.NewFakeK8sClientFromObjects(hr)
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

	updated := &helmv2.HelmRelease{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testResourceName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("failed to get HelmRelease: %v", err)
	}
	if len(updated.Spec.ValuesFrom) != 0 {
		t.Errorf("expected ValuesFrom unchanged (empty), got %d entries", len(updated.Spec.ValuesFrom))
	}
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
