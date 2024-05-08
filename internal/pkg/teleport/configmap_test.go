package teleport

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func Test_ConfigMapCRUD(t *testing.T) {
	testCases := []struct {
		name              string
		namespace         string
		clusterName       string
		registerName      string
		token             string
		configMap         *corev1.ConfigMap
		configMapToCreate *corev1.ConfigMap
		configMapToRead   *corev1.ConfigMap
		configMapToUpdate *corev1.ConfigMap
		configMapToDelete *corev1.ConfigMap
		expectError       bool
		expectEmpty       bool
		config            *config.Config
	}{
		{
			name:              "case 0: Create a configmap if it does not exist",
			namespace:         test.NamespaceName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			configMapToCreate: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:              "case 1: Successfully finish creation of a configmap if it already exists",
			namespace:         test.NamespaceName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			configMap:         test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			configMapToCreate: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectError:       false,
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:            "case 2: Read an existing configmap",
			namespace:       test.NamespaceName,
			clusterName:     test.ClusterName,
			registerName:    key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			configMap:       test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			configMapToRead: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:            "case 3: Succeed when reading a non-existent configmap",
			namespace:       test.NamespaceName,
			clusterName:     test.ClusterName,
			registerName:    key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			configMapToRead: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectEmpty:     true,
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:            "case 4: Read token from an existing configmap",
			namespace:       test.NamespaceName,
			clusterName:     test.ClusterName,
			registerName:    key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:           test.TokenName,
			configMap:       test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			configMapToRead: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:         "case 5: Fail to read token from an invalid configmap",
			namespace:    test.NamespaceName,
			clusterName:  test.ClusterName,
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:        test.TokenName,
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.GetConfigmapName(test.ClusterName, test.AppName),
					Namespace: test.NamespaceName,
				},
			},
			configMapToRead: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectError:     true,
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:              "case 6: Update an existing configmap",
			namespace:         test.NamespaceName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:             test.NewTokenName,
			configMap:         test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			configMapToUpdate: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.NewTokenName),
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:              "case 7: Delete an existing configmap",
			namespace:         test.NamespaceName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			configMap:         test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			configMapToDelete: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectError:       false,
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
		{
			name:              "case 8: Succeed when deleting a non-existent configmap",
			namespace:         test.NamespaceName,
			clusterName:       test.ClusterName,
			registerName:      key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			configMapToDelete: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, test.TokenName),
			expectError:       false,
			config: &config.Config{
				AppName:         test.AppName,
				ProxyAddr:       test.ProxyAddr,
				TeleportVersion: test.TeleportVersion,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualConfigMap *corev1.ConfigMap

			var runtimeObjects []runtime.Object
			if tc.configMap != nil {
				runtimeObjects = append(runtimeObjects, tc.configMap)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			teleport := New(tc.name, tc.config, token.NewGenerator())

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			if tc.configMapToRead != nil {
				actualConfigMap, err = teleport.GetConfigMap(ctx, log, ctrlClient, tc.clusterName, tc.namespace)
				test.CheckError(t, tc.expectError && tc.token == "", err)
				if err == nil {
					if tc.token != "" {
						var actualToken string
						actualToken, err = teleport.GetTokenFromConfigMap(ctx, actualConfigMap)
						test.CheckError(t, tc.expectError, err)
						if err == nil && tc.token != actualToken {
							t.Fatalf("unexpected token: expected %s, actual %s", tc.token, actualToken)
						}
					} else if tc.expectEmpty && actualConfigMap != nil {
						t.Fatalf("unexpected result: expected nil, actual %v", actualConfigMap)
					} else if !tc.expectEmpty {
						test.CheckConfigMap(t, tc.configMapToRead, actualConfigMap)
					}
				}
			}

			if tc.configMapToCreate != nil {
				err = teleport.CreateConfigMap(ctx, log, ctrlClient, tc.clusterName, tc.namespace, tc.registerName, tc.token)
				test.CheckError(t, tc.expectError, err)
				if err != nil {
					actualConfigMap, err = loadConfigMap(ctx, ctrlClient, tc.configMapToCreate)
					test.CheckError(t, false, err)
					if err != nil {
						test.CheckConfigMap(t, tc.configMapToCreate, actualConfigMap)
					}
				}
			}

			if tc.configMapToUpdate != nil {
				err = teleport.UpdateConfigMap(ctx, log, ctrlClient, tc.configMap, tc.token)
				test.CheckError(t, tc.expectError, err)
				if err != nil {
					actualConfigMap, err = loadConfigMap(ctx, ctrlClient, tc.configMapToUpdate)
					test.CheckError(t, false, err)
					if err != nil {
						test.CheckConfigMap(t, tc.configMapToUpdate, actualConfigMap)
					}
					if actualConfigMap.Labels["app-operator.giantswarm.io/watching"] != "false" {
						t.Errorf("Expected label app-operator.giantswarm.io/watching=false, found %s", actualConfigMap.Labels["app-operator.giantswarm.io/watching"])
					}
				}
			}

			if tc.configMapToDelete != nil {
				err = teleport.DeleteConfigMap(ctx, log, ctrlClient, tc.clusterName, tc.namespace)
				test.CheckError(t, tc.expectError, err)
				if err == nil {
					_, err = loadConfigMap(ctx, ctrlClient, tc.configMapToDelete)
					if err != nil && !errors.IsNotFound(err) {
						t.Fatalf("unexpected error %v", err)
					}
					if err == nil {
						t.Fatalf("unexpected result: config map %v is present in the cluster", tc.configMapToDelete)
					}
				}
			}
		})
	}
}

func loadConfigMap(ctx context.Context, ctrlClient client.Client, expected *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	actual := &corev1.ConfigMap{}
	err := ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(expected.ObjectMeta), actual)
	if err != nil {
		return nil, err
	}
	return actual, err
}
