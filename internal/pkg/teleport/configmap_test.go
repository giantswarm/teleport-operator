package teleport

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func Test_CreateConfigMap(t *testing.T) {
	testCases := []struct {
		name              string
		namespace         string
		appName           string
		clusterName       string
		registerName      string
		token             string
		configMap         *corev1.ConfigMap
		expectedConfigMap *corev1.ConfigMap
	}{
		{
			name:              "case 0: Create config map if it does not exist",
			namespace:         test.NamespaceName,
			appName:           test.AppName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:             test.TokenName,
			expectedConfigMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
		},
		{
			name:              "case 1: Do nothing in case the config map already exists",
			namespace:         test.NamespaceName,
			appName:           test.AppName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:             test.NewTokenName,
			configMap:         test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectedConfigMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.configMap != nil {
				runtimeObjects = append(runtimeObjects, tc.configMap)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			secretConfig := &SecretConfig{
				AppName:         tc.appName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			}

			teleport := New(tc.namespace, secretConfig, token.NewGenerator())
			err = teleport.CreateConfigMap(ctx, log, ctrlClient, tc.clusterName, tc.registerName, tc.namespace, tc.token)
			test.CheckError(t, false, err)

			actual := &corev1.ConfigMap{}
			err = ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(tc.expectedConfigMap.ObjectMeta), actual)
			test.CheckError(t, false, err)
			test.CheckConfigMap(t, tc.expectedConfigMap, actual)
		})
	}
}

func Test_DeleteConfigMap(t *testing.T) {
	testCases := []struct {
		name          string
		namespace     string
		appName       string
		clusterName   string
		configMap     *corev1.ConfigMap
		expectDeleted bool
	}{
		{
			name:          "case 0: Delete config map in case it exists",
			namespace:     test.NamespaceName,
			appName:       test.AppName,
			clusterName:   test.ClusterName,
			configMap:     test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectDeleted: true,
		},
		{
			name:          "case 1: Succeed in case the config map does not exist",
			namespace:     test.NamespaceName,
			appName:       test.AppName,
			clusterName:   test.ManagementClusterName,
			configMap:     test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectDeleted: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.configMap != nil {
				runtimeObjects = append(runtimeObjects, tc.configMap)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			secretConfig := &SecretConfig{
				AppName:         tc.appName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			}

			teleport := New(tc.namespace, secretConfig, token.NewGenerator())
			err = teleport.DeleteConfigMap(ctx, log, ctrlClient, tc.clusterName, tc.namespace)
			test.CheckError(t, false, err)

			actual := &corev1.ConfigMap{}
			err = ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(tc.configMap.ObjectMeta), actual)

			if err != nil && !errors.IsNotFound(err) {
				t.Fatalf("unexpected error %v", err)
			}

			isDeleted := err != nil
			if tc.expectDeleted != isDeleted {
				t.Fatalf("unexpected deletion result, expected %v, ectual %v", tc.expectDeleted, isDeleted)
			}
		})
	}
}
