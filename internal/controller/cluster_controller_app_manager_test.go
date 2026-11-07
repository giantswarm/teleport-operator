package controller

import (
	"context"
	"testing"
	"time"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
)

// reconcileWithBot is a helper that runs a full reconcile with IsBotEnabled: true
// and returns the fake client so callers can inspect state.
func reconcileWithBot(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	fakeClient, err := test.NewFakeK8sClientFromObjects(objects...)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	controller := &ClusterReconciler{
		Client:    fakeClient,
		Log:       log,
		Scheme:    scheme.Scheme,
		Namespace: test.NamespaceName,
		Teleport: teleport.New(
			test.NamespaceName,
			&config.Config{
				AppName:               test.AppName,
				AppCatalog:            test.AppCatalog,
				AppVersion:            test.AppVersion,
				ManagementClusterName: test.ManagementClusterName,
				ProxyAddr:             test.ProxyAddr,
				TeleportVersion:       test.TeleportVersion,
			},
			test.NewMockTokenGenerator(test.TokenName),
		),
		IsBotEnabled: true,
	}
	controller.Teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{})
	controller.Teleport.Identity = &config.IdentityConfig{
		IdentityFile: test.IdentityFileValue,
		LastRead:     time.Now(),
	}
	controller.Teleport.Client = fakeClient

	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	_, err = controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}

	return fakeClient
}

// reconcileDeleteWithBot runs a reconcile for a cluster marked for deletion with IsBotEnabled: true.
func reconcileDeleteWithBot(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	fakeClient, err := test.NewFakeK8sClientFromObjects(objects...)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	controller := &ClusterReconciler{
		Client:    fakeClient,
		Log:       log,
		Scheme:    scheme.Scheme,
		Namespace: test.NamespaceName,
		Teleport: teleport.New(
			test.NamespaceName,
			&config.Config{
				AppName:               test.AppName,
				AppCatalog:            test.AppCatalog,
				AppVersion:            test.AppVersion,
				ManagementClusterName: test.ManagementClusterName,
				ProxyAddr:             test.ProxyAddr,
				TeleportVersion:       test.TeleportVersion,
			},
			test.NewMockTokenGenerator(test.TokenName),
		),
		IsBotEnabled: true,
	}
	controller.Teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{})
	controller.Teleport.Identity = &config.IdentityConfig{
		IdentityFile: test.IdentityFileValue,
		LastRead:     time.Now(),
	}
	controller.Teleport.Client = fakeClient

	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now())
	_, err = controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}

	return fakeClient
}

// --- tbot tests (cases A-E) ---

// case A: tbot App CR exists — EnsureConfig adds ExtraConfigs entry.
func Test_ClusterController_BotEnabled_TbotAppCR_EnsureConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	tbotApp := test.NewApp(key.TeleportBotAppName, key.TeleportBotNamespace)

	fakeClient := reconcileWithBot(t, cluster, tbotApp)

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: key.TeleportBotAppName, Namespace: key.TeleportBotNamespace},
		updated); err != nil {
		t.Fatalf("failed to get tbot App: %v", err)
	}

	wantCM := key.GetTbotConfigmapName(test.ClusterName)
	for _, cfg := range updated.Spec.ExtraConfigs {
		if cfg.Name == wantCM {
			return // found — test passes
		}
	}
	t.Errorf("expected ExtraConfigs to contain %q, got %+v", wantCM, updated.Spec.ExtraConfigs)
}

// case B: tbot HelmRelease exists — the operator leaves its valuesFrom alone.
func Test_ClusterController_BotEnabled_TbotHelmRelease_EnsureConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	tbotHR := test.NewHelmRelease(key.TeleportBotAppName, key.TeleportBotNamespace)

	fakeClient := reconcileWithBot(t, cluster, tbotHR)

	updated := test.NewHelmRelease(key.TeleportBotAppName, key.TeleportBotNamespace)
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: key.TeleportBotAppName, Namespace: key.TeleportBotNamespace},
		updated); err != nil {
		t.Fatalf("failed to get tbot HelmRelease: %v", err)
	}

	if valuesFrom := helmReleaseValuesFrom(updated); len(valuesFrom) != 0 {
		t.Errorf("expected the HelmRelease valuesFrom to be left empty, got %+v", valuesFrom)
	}
}

// case C: neither tbot App CR nor HelmRelease exists — reconcile succeeds (no-op).
func Test_ClusterController_BotEnabled_NoTbotResource_NoOp(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	// No tbot resource — reconcile should succeed without error.
	reconcileWithBot(t, cluster)
}

// case D: tbot App CR exists, cluster deleting — DeleteConfig removes ExtraConfigs entry.
func Test_ClusterController_BotEnabled_TbotAppCR_DeleteConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now())
	tbotApp := test.NewApp(key.TeleportBotAppName, key.TeleportBotNamespace)
	tbotApp.Spec.ExtraConfigs = []appv1alpha1.AppExtraConfig{
		{Kind: "configMap", Name: key.GetTbotConfigmapName(test.ClusterName), Namespace: key.TeleportBotNamespace, Priority: 25},
	}

	fakeClient := reconcileDeleteWithBot(t, cluster, tbotApp)

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: key.TeleportBotAppName, Namespace: key.TeleportBotNamespace},
		updated); err != nil {
		t.Fatalf("failed to get tbot App: %v", err)
	}

	wantCM := key.GetTbotConfigmapName(test.ClusterName)
	for _, cfg := range updated.Spec.ExtraConfigs {
		if cfg.Name == wantCM {
			t.Errorf("expected ExtraConfigs entry %q to be removed, still present", wantCM)
		}
	}
}

// case E: tbot HelmRelease exists, cluster deleting — its valuesFrom is left alone.
func Test_ClusterController_BotEnabled_TbotHelmRelease_DeleteConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now())
	tbotHR := test.NewHelmRelease(key.TeleportBotAppName, key.TeleportBotNamespace)
	tbotHR.Object["spec"] = map[string]interface{}{
		"valuesFrom": []interface{}{
			map[string]interface{}{
				"kind":      "ConfigMap",
				"name":      key.GetTbotConfigmapName(test.ClusterName),
				"valuesKey": "values",
			},
		},
	}

	fakeClient := reconcileDeleteWithBot(t, cluster, tbotHR)

	updated := test.NewHelmRelease(key.TeleportBotAppName, key.TeleportBotNamespace)
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: key.TeleportBotAppName, Namespace: key.TeleportBotNamespace},
		updated); err != nil {
		t.Fatalf("failed to get tbot HelmRelease: %v", err)
	}

	if valuesFrom := helmReleaseValuesFrom(updated); len(valuesFrom) != 1 {
		t.Errorf("expected the HelmRelease valuesFrom to be left alone, got %+v", valuesFrom)
	}
}

// --- kube-agent tests (cases F-I) ---

// reconcileWithKubeAgent runs a reconcile without bot enabled, focused on kube-agent injection.
func reconcileWithKubeAgent(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	fakeClient, err := test.NewFakeK8sClientFromObjects(objects...)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	controller := &ClusterReconciler{
		Client:    fakeClient,
		Log:       log,
		Scheme:    scheme.Scheme,
		Namespace: test.NamespaceName,
		Teleport: teleport.New(
			test.NamespaceName,
			&config.Config{
				AppName:               test.AppName,
				AppCatalog:            test.AppCatalog,
				AppVersion:            test.AppVersion,
				ManagementClusterName: test.ManagementClusterName,
				ProxyAddr:             test.ProxyAddr,
				TeleportVersion:       test.TeleportVersion,
			},
			test.NewMockTokenGenerator(test.TokenName),
		),
		IsBotEnabled: false,
	}
	controller.Teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{})
	controller.Teleport.Identity = &config.IdentityConfig{
		IdentityFile: test.IdentityFileValue,
		LastRead:     time.Now(),
	}
	controller.Teleport.Client = fakeClient

	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	_, err = controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}

	return fakeClient
}

// reconcileDeleteWithKubeAgent runs a delete reconcile without bot enabled.
func reconcileDeleteWithKubeAgent(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	fakeClient, err := test.NewFakeK8sClientFromObjects(objects...)
	if err != nil {
		t.Fatalf("failed to create fake client: %v", err)
	}

	ctx := context.TODO()
	log := ctrl.Log.WithName("test")

	controller := &ClusterReconciler{
		Client:    fakeClient,
		Log:       log,
		Scheme:    scheme.Scheme,
		Namespace: test.NamespaceName,
		Teleport: teleport.New(
			test.NamespaceName,
			&config.Config{
				AppName:               test.AppName,
				AppCatalog:            test.AppCatalog,
				AppVersion:            test.AppVersion,
				ManagementClusterName: test.ManagementClusterName,
				ProxyAddr:             test.ProxyAddr,
				TeleportVersion:       test.TeleportVersion,
			},
			test.NewMockTokenGenerator(test.TokenName),
		),
		IsBotEnabled: false,
	}
	controller.Teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{})
	controller.Teleport.Identity = &config.IdentityConfig{
		IdentityFile: test.IdentityFileValue,
		LastRead:     time.Now(),
	}
	controller.Teleport.Client = fakeClient

	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now())
	_, err = controller.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace},
	})
	if err != nil {
		t.Fatalf("reconcile returned unexpected error: %v", err)
	}

	return fakeClient
}

// kubeAgentAppName returns the per-cluster kube-agent app name used in tests.
func kubeAgentAppName() string {
	return key.GetAppName(test.ClusterName, test.AppName)
}

// case F: kube-agent App CR exists — EnsureConfig adds ExtraConfigs entry.
func Test_ClusterController_KubeAgent_AppCR_EnsureConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	kubeApp := test.NewApp(kubeAgentAppName(), test.NamespaceName)

	fakeClient := reconcileWithKubeAgent(t, cluster, kubeApp)

	updated := &appv1alpha1.App{}
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: kubeAgentAppName(), Namespace: test.NamespaceName},
		updated); err != nil {
		t.Fatalf("failed to get kube-agent App: %v", err)
	}

	wantCM := key.GetConfigmapName(test.ClusterName, test.AppName)
	for _, cfg := range updated.Spec.ExtraConfigs {
		if cfg.Name == wantCM {
			return
		}
	}
	t.Errorf("expected ExtraConfigs to contain %q, got %+v", wantCM, updated.Spec.ExtraConfigs)
}

// case G: kube-agent HelmRelease exists — the operator leaves its valuesFrom alone.
func Test_ClusterController_KubeAgent_HelmRelease_EnsureConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	kubeHR := test.NewHelmRelease(kubeAgentAppName(), test.NamespaceName)

	fakeClient := reconcileWithKubeAgent(t, cluster, kubeHR)

	updated := test.NewHelmRelease(kubeAgentAppName(), test.NamespaceName)
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: kubeAgentAppName(), Namespace: test.NamespaceName},
		updated); err != nil {
		t.Fatalf("failed to get kube-agent HelmRelease: %v", err)
	}

	if valuesFrom := helmReleaseValuesFrom(updated); len(valuesFrom) != 0 {
		t.Errorf("expected the HelmRelease valuesFrom to be left empty, got %+v", valuesFrom)
	}
}

// case H: neither kube-agent resource exists — reconcile succeeds (no-op).
func Test_ClusterController_KubeAgent_NoResource_NoOp(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	reconcileWithKubeAgent(t, cluster)
}

// case I: kube-agent HelmRelease exists, cluster deleting — its valuesFrom is left alone.
func Test_ClusterController_KubeAgent_HelmRelease_DeleteConfig(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now())
	kubeHR := test.NewHelmRelease(kubeAgentAppName(), test.NamespaceName)
	kubeHR.Object["spec"] = map[string]interface{}{
		"valuesFrom": []interface{}{
			map[string]interface{}{
				"kind":      "ConfigMap",
				"name":      key.GetConfigmapName(test.ClusterName, test.AppName),
				"valuesKey": "values",
			},
		},
	}
	// Also provide the existing secret/configmap so deletion path runs cleanly.
	secret := test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName)
	configMap := test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName, []string{key.RoleKube})

	fakeClient := reconcileDeleteWithKubeAgent(t, cluster, kubeHR, secret, configMap)

	updated := test.NewHelmRelease(kubeAgentAppName(), test.NamespaceName)
	if err := fakeClient.Get(context.TODO(),
		client.ObjectKey{Name: kubeAgentAppName(), Namespace: test.NamespaceName},
		updated); err != nil {
		t.Fatalf("failed to get kube-agent HelmRelease: %v", err)
	}

	if valuesFrom := helmReleaseValuesFrom(updated); len(valuesFrom) != 1 {
		t.Errorf("expected the HelmRelease valuesFrom to be left alone, got %+v", valuesFrom)
	}
}

// identitySecretInGiantswarm returns an identity secret in the giantswarm namespace,
// needed for the tbot path which reads the kubeconfig secret from there.
func identitySecretInGiantswarm() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.TeleportBotSecretName,
			Namespace: key.TeleportBotNamespace,
		},
		Data: map[string][]byte{key.Identity: []byte(test.IdentityFileValue)},
	}
}

func helmReleaseValuesFrom(hr *unstructured.Unstructured) []interface{} {
	valuesFrom, _, _ := unstructured.NestedSlice(hr.Object, "spec", "valuesFrom")
	return valuesFrom
}

// Every cluster is registered, including one that is being deleted.
func Test_ClusterController_BotEnabled_RegistersAllClusters(t *testing.T) {
	cluster := test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{})
	deleting := test.NewCluster("badger", test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now())
	tbotApp := test.NewApp(key.TeleportBotAppName, key.TeleportBotNamespace)

	fakeClient := reconcileWithBot(t, cluster, deleting, tbotApp)

	outputs := test.ReadTbotRegisteredClusters(t, context.TODO(), fakeClient)
	want := map[string]string{
		key.RegisterName(test.ManagementClusterName, test.ClusterName): test.ClusterName,
		key.RegisterName(test.ManagementClusterName, "badger"):         "badger",
	}
	for reg, cl := range want {
		if outputs[reg] != cl {
			t.Errorf("expected outputs[%s]=%s, got outputs %v", reg, cl, outputs)
		}
	}
}

// Once a Cluster is gone, its entry and any stale entry are removed.
func Test_ClusterController_BotEnabled_GoneClusterIsUnregistered(t *testing.T) {
	survivor := test.NewCluster("badger", test.NamespaceName, nil, time.Time{})
	registered := test.NewTbotRegisteredClustersConfigMap(map[string]string{
		key.RegisterName(test.ManagementClusterName, test.ClusterName): test.ClusterName,
		key.RegisterName(test.ManagementClusterName, "badger"):         "badger",
		"golem-orphan": "orphan",
	})

	// test.ClusterName does not exist, so the reconcile takes the NotFound path.
	fakeClient := reconcileWithBot(t, survivor, registered)

	outputs := test.ReadTbotRegisteredClusters(t, context.TODO(), fakeClient)
	want := map[string]interface{}{key.RegisterName(test.ManagementClusterName, "badger"): "badger"}
	if len(outputs) != len(want) || outputs[key.RegisterName(test.ManagementClusterName, "badger")] != "badger" {
		t.Errorf("expected only the surviving cluster, got %v", outputs)
	}
}
