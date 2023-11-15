package config

import (
	"context"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func Test_GetConfigFromSecret(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		configMap      *corev1.ConfigMap
		testSecret     bool
		testConfigMap  bool
		expectedConfig *Config
		expectError    bool
	}{
		{
			name:      "case 0: Return config in case a valid config map exists",
			namespace: test.NamespaceName,
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.TeleportOperatorConfigName,
					Namespace: test.NamespaceName,
				},
				Data: map[string]string{
					key.AppCatalog:            test.AppCatalog,
					key.AppName:               test.AppName,
					key.AppVersion:            test.AppVersion,
					key.ManagementClusterName: test.ManagementClusterName,
					key.ProxyAddr:             test.ProxyAddr,
					key.TeleportVersion:       test.TeleportVersion,
				},
			},
			testConfigMap: true,
			expectedConfig: &Config{
				AppCatalog:            test.AppCatalog,
				AppName:               test.AppName,
				AppVersion:            test.AppVersion,
				ManagementClusterName: test.ManagementClusterName,
				ProxyAddr:             test.ProxyAddr,
				TeleportVersion:       test.TeleportVersion,
			},
		},
		{
			name:          "case 1: Fail in case the config map does not exist",
			namespace:     test.NamespaceName,
			testConfigMap: true,
			expectError:   true,
		},
		{
			name:      "case 2: Fail in case the config map exists but does not contain all keys",
			namespace: test.NamespaceName,
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.TeleportOperatorConfigName,
					Namespace: test.NamespaceName,
				},
				Data: map[string]string{},
			},
			testConfigMap: true,
			expectError:   true,
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

			var actualConfig *Config
			actualConfig, err = GetConfigFromConfigMap(ctx, ctrlClient, tc.namespace)
			test.CheckError(t, tc.expectError, err)

			if err == nil {
				checkConfigs(t, tc.expectedConfig, actualConfig)
			}
		})
	}
}

func checkConfigs(t *testing.T, expected *Config, actual *Config) {
	configsMatch := expected.AppVersion == actual.AppVersion &&
		expected.AppName == actual.AppName &&
		expected.AppCatalog == actual.AppCatalog &&
		expected.ManagementClusterName == actual.ManagementClusterName &&
		expected.ProxyAddr == actual.ProxyAddr &&
		expected.TeleportVersion == actual.TeleportVersion

	if !configsMatch {
		t.Fatalf("configs do not match: expected\n%v,\nactual\n%v", expected, actual)
	}
}
