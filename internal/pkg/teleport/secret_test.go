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

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func Test_GetConfigFromSecret(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		secret         *corev1.Secret
		expectedConfig *SecretConfig
		expectError    bool
	}{
		{
			name:      "case 0: Return config in case a valid secret exists",
			namespace: test.NamespaceName,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.TeleportOperatorSecretName,
					Namespace: test.NamespaceName,
				},
				Data: map[string][]byte{
					key.AppCatalog:            []byte(test.AppCatalog),
					key.AppName:               []byte(test.AppName),
					key.AppVersion:            []byte(test.AppVersion),
					key.IdentityFile:          []byte(test.IdentityFileValue),
					key.ManagementClusterName: []byte(test.ManagementClusterName),
					key.ProxyAddr:             []byte(test.ProxyAddr),
					key.TeleportVersion:       []byte(test.TeleportVersion),
				},
			},
			expectedConfig: &SecretConfig{
				AppCatalog:            test.AppCatalog,
				AppName:               test.AppName,
				AppVersion:            test.AppVersion,
				ManagementClusterName: test.ManagementClusterName,
				IdentityFile:          test.IdentityFileValue,
				ProxyAddr:             test.ProxyAddr,
				TeleportVersion:       test.TeleportVersion,
			},
		},
		{
			name:        "case 1: Fail in case the config secret does not exist",
			namespace:   test.NamespaceName,
			expectError: true,
		},
		{
			name:      "case 2: Fail in case the config secret exists but does not contain all keys",
			namespace: test.NamespaceName,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.TeleportOperatorSecretName,
					Namespace: test.NamespaceName,
				},
				Data: map[string][]byte{},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.secret != nil {
				runtimeObjects = append(runtimeObjects, tc.secret)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			actualConfig, err := GetConfigFromSecret(ctx, ctrlClient, tc.namespace)
			test.CheckError(t, tc.expectError, err)

			if err == nil {
				configsMatch := tc.expectedConfig.AppVersion == actualConfig.AppVersion &&
					tc.expectedConfig.AppName == actualConfig.AppName &&
					tc.expectedConfig.AppCatalog == actualConfig.AppCatalog &&
					tc.expectedConfig.IdentityFile == actualConfig.IdentityFile &&
					tc.expectedConfig.ManagementClusterName == actualConfig.ManagementClusterName &&
					tc.expectedConfig.ProxyAddr == actualConfig.ProxyAddr &&
					tc.expectedConfig.TeleportVersion == actualConfig.TeleportVersion

				if !configsMatch {
					t.Fatalf("configs do not match: expected\n%v,\nactual\n%v", tc.expectedConfig, actualConfig)
				}
			}
		})
	}
}

func Test_SecretCRUD(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		clusterName    string
		token          string
		secret         *corev1.Secret
		secretToCreate *corev1.Secret
		secretToRead   *corev1.Secret
		secretToUpdate *corev1.Secret
		secretToDelete *corev1.Secret
		expectError    bool
		expectEmpty    bool
	}{
		{
			name:           "case 0: Create a secret if it does not exist",
			namespace:      test.NamespaceName,
			clusterName:    test.ClusterName,
			token:          test.TokenName,
			secretToCreate: test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
		},
		{
			name:           "case 1: Fail to create secret in case it already exists",
			namespace:      test.NamespaceName,
			clusterName:    test.ClusterName,
			token:          test.TokenName,
			secret:         test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			secretToCreate: test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			expectError:    true,
		},
		{
			name:        "case 2: Read an existing secret",
			namespace:   test.NamespaceName,
			clusterName: test.ClusterName,
			secret:      test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
		},
		{
			name:         "case 3: Read join token from an existing secret",
			namespace:    test.NamespaceName,
			clusterName:  test.ClusterName,
			secret:       test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			secretToRead: test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
		},
		{
			name:        "case 4: Fail to read join token from an invalid existing secret",
			namespace:   test.NamespaceName,
			clusterName: test.ClusterName,
			token:       test.TokenName,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.GetSecretName(test.ClusterName),
					Namespace: test.NamespaceName,
				},
			},
			secretToRead: test.NewSecret(test.ClusterName, test.NamespaceName, ""),
			expectError:  true,
		},
		{
			name:         "case 5: Succeed when reading a non-existent secret",
			namespace:    test.NamespaceName,
			clusterName:  test.ClusterName,
			secretToRead: test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			expectError:  false,
			expectEmpty:  true,
		},
		{
			name:           "case 6: Update an existing secret",
			namespace:      test.NamespaceName,
			clusterName:    test.ClusterName,
			token:          test.NewTokenName,
			secret:         test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			secretToUpdate: test.NewSecret(test.ClusterName, test.NamespaceName, test.NewTokenName),
		},
		{
			name:           "case 7: Fail to update a non-existent secret",
			namespace:      test.NamespaceName,
			clusterName:    test.ClusterName,
			token:          test.NewTokenName,
			secretToUpdate: test.NewSecret(test.ClusterName, test.NamespaceName, test.NewTokenName),
			expectError:    true,
		},
		{
			name:           "case 8: Delete an existing secret",
			namespace:      test.NamespaceName,
			clusterName:    test.ClusterName,
			secret:         test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			secretToDelete: test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
		},
		{
			name:           "case 9: Succeed when deleting a non-existent secret",
			namespace:      test.NamespaceName,
			clusterName:    test.ClusterName,
			secretToDelete: test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var actualSecret *corev1.Secret

			var runtimeObjects []runtime.Object
			if tc.secret != nil {
				runtimeObjects = append(runtimeObjects, tc.secret)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			teleport := New(tc.name, &SecretConfig{}, token.NewGenerator())

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			if tc.secretToRead != nil {
				actualSecret, err = teleport.GetSecret(ctx, log, ctrlClient, tc.clusterName, tc.namespace)
				test.CheckError(t, tc.expectError && tc.token == "", err)
				if err == nil {
					if tc.token != "" {
						var actualToken string
						actualToken, err = teleport.GetTokenFromSecret(ctx, actualSecret)
						test.CheckError(t, tc.expectError, err)
						if err == nil && tc.token != actualToken {
							t.Fatalf("unexpected token: expected %s, actual %s", tc.token, actualToken)
						}
					} else if tc.expectEmpty && actualSecret != nil {
						t.Fatalf("unexpected result: expected nil, actual %v", actualSecret)
					} else if !tc.expectEmpty {
						test.CheckSecret(t, tc.secretToRead, actualSecret)
					}
				}
			}

			if tc.secretToCreate != nil {
				err = teleport.CreateSecret(ctx, log, ctrlClient, tc.clusterName, tc.namespace, tc.token)
				test.CheckError(t, tc.expectError, err)
				if err == nil {
					actualSecret, err = loadSecret(ctx, ctrlClient, tc.secretToCreate)
					test.CheckError(t, false, err)
					if err == nil {
						test.CheckSecret(t, tc.secretToCreate, actualSecret)
					}
				}
			}

			if tc.secretToUpdate != nil {
				err = teleport.UpdateSecret(ctx, log, ctrlClient, tc.clusterName, tc.namespace, tc.token)
				test.CheckError(t, tc.expectError, err)
				if err == nil {
					actualSecret, err = loadSecret(ctx, ctrlClient, tc.secretToUpdate)
					test.CheckError(t, false, err)
					if err == nil {
						test.CheckSecret(t, tc.secretToUpdate, actualSecret)
					}
				}
			}

			if tc.secretToDelete != nil {
				err = teleport.DeleteSecret(ctx, log, ctrlClient, tc.clusterName, tc.namespace)
				test.CheckError(t, tc.expectError, err)
				if err == nil {
					_, err = loadSecret(ctx, ctrlClient, tc.secretToDelete)
					if err != nil && !errors.IsNotFound(err) {
						t.Fatalf("unexpected error %v", err)
					}
					if err == nil {
						t.Fatalf("unexpected result: secret %v is present in the clsuter", tc.secretToDelete)
					}
				}
			}
		})
	}
}

func loadSecret(ctx context.Context, ctrlClient client.Client, expected *corev1.Secret) (*corev1.Secret, error) {
	actual := &corev1.Secret{}
	err := ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(expected.ObjectMeta), actual)
	if err != nil {
		return nil, err
	}
	return actual, nil
}
