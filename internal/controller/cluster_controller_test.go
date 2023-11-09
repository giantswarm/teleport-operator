package controller

import (
	"context"
	"testing"
	"time"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	teleportTypes "github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
)

func Test_ClusterController(t *testing.T) {
	testCases := []struct {
		name              string
		namespace         string
		token             string
		tokens            []teleportTypes.ProvisionToken
		secretConfig      *teleport.SecretConfig
		cluster           *capi.Cluster
		secret            *corev1.Secret
		configMap         *corev1.ConfigMap
		expectedCluster   *capi.Cluster
		expectedSecret    *corev1.Secret
		expectedConfigMap *corev1.ConfigMap
	}{
		{
			name:              "case 0: Register cluster and create Secret, ConfigMap and App resources in case they do not exist",
			namespace:         test.NamespaceName,
			token:             test.TokenName,
			secretConfig:      newSecretConfig(),
			cluster:           test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			expectedCluster:   test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			expectedSecret:    test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			expectedConfigMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
		},
		{
			name:              "case 1: Register cluster and update Secret, ConfigMap and App resources in case they exist",
			namespace:         test.NamespaceName,
			token:             test.TokenName,
			secretConfig:      newSecretConfig(),
			cluster:           test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			secret:            test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			configMap:         test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectedCluster:   test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			expectedSecret:    test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			expectedConfigMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
		},
		{
			name:              "case 2: Update Secret and ConfigMap resources in case join token changes",
			namespace:         test.NamespaceName,
			token:             test.NewTokenName,
			secretConfig:      newSecretConfig(),
			cluster:           test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			secret:            test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			configMap:         test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectedCluster:   test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			expectedSecret:    test.NewSecret(test.ClusterName, test.NamespaceName, test.NewTokenName),
			expectedConfigMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.NewTokenName),
		},
		{
			name:         "case 3: Deregister cluster and delete resources in case the cluster is deleted",
			namespace:    test.NamespaceName,
			token:        test.TokenName,
			secretConfig: newSecretConfig(),
			cluster:      test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now()),
			secret:       test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			configMap:    test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.cluster != nil {
				runtimeObjects = append(runtimeObjects, tc.cluster)
			}
			if tc.secret != nil {
				runtimeObjects = append(runtimeObjects, tc.secret)
			}
			if tc.configMap != nil {
				runtimeObjects = append(runtimeObjects, tc.configMap)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			controller := &ClusterReconciler{
				Client:    ctrlClient,
				Log:       log,
				Scheme:    scheme.Scheme,
				Namespace: tc.namespace,
				Teleport:  teleport.New(tc.namespace, tc.secretConfig, test.NewMockTokenGenerator(tc.token)),
			}
			controller.Teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				Tokens: tc.tokens,
			})

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tc.cluster.Name,
					Namespace: tc.cluster.Namespace,
				},
			}

			_, err = controller.Reconcile(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			clusterList := &capi.ClusterList{}
			err = ctrlClient.List(ctx, clusterList, client.InNamespace(tc.namespace))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.expectedCluster != nil {
				if len(clusterList.Items) == 0 {
					t.Fatalf("unexpected result: cluster list is empty\n%v", clusterList)
				}
				test.CheckCluster(t, tc.expectedCluster, &clusterList.Items[0])
			} else if len(clusterList.Items) > 0 {
				t.Fatalf("unexpected result: cluster list is not empty\n%v", clusterList)
			}

			secretList := &corev1.SecretList{}
			err = ctrlClient.List(ctx, secretList, client.InNamespace(tc.namespace))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.expectedSecret != nil {
				if len(secretList.Items) == 0 {
					t.Fatalf("unexpected result: secret list is empty\n%v", secretList)
				}
				test.CheckSecret(t, tc.expectedSecret, &secretList.Items[0])
			} else if len(secretList.Items) > 0 {
				t.Fatalf("unexpected result: secret list is not empty\n%v", secretList)
			}

			configMapList := &corev1.ConfigMapList{}
			err = ctrlClient.List(ctx, configMapList, client.InNamespace(tc.namespace))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.expectedConfigMap != nil {
				if len(configMapList.Items) == 0 {
					t.Fatalf("unexpected result: secret list is empty\n%v", secretList)
				}
				test.CheckConfigMap(t, tc.expectedConfigMap, &configMapList.Items[0])
			} else if len(configMapList.Items) > 0 {
				t.Fatalf("unexpected result: config map list is not empty\n%v", secretList)
			}

			appList := &appv1alpha1.AppList{}
			err = ctrlClient.List(ctx, appList, client.InNamespace(tc.namespace))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}

func newSecretConfig() *teleport.SecretConfig {
	return &teleport.SecretConfig{
		AppCatalog:            test.AppCatalog,
		AppName:               test.AppName,
		AppVersion:            test.AppVersion,
		IdentityFile:          test.IdentityFileValue,
		LastRead:              test.LastReadValue,
		ManagementClusterName: test.ManagementClusterName,
		ProxyAddr:             test.ProxyAddr,
		TeleportVersion:       test.TeleportVersion,
	}
}
