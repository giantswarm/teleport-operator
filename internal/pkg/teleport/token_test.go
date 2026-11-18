package teleport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func Test_GenerateToken(t *testing.T) {
	testCases := []struct {
		name           string
		registerName   string
		tokenType      []string
		failsCreate    bool
		expectError    bool
		expectedRoles  []string
		expectedExpiry time.Duration
	}{
		{
			name:           "case 0: Generate a new kube token",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenType:      []string{"kube"},
			expectError:    false,
			expectedRoles:  []string{"kube"},
			expectedExpiry: 720 * time.Hour,
		},
		{
			name:           "case 1: Generate a new node token",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenType:      []string{"node"},
			expectError:    false,
			expectedRoles:  []string{"node"},
			expectedExpiry: 720 * time.Hour,
		},
		{
			name:           "case 2: Generate a new kube and app token",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenType:      []string{"kube", "app"},
			expectError:    false,
			expectedRoles:  []string{"kube", "app"},
			expectedExpiry: 720 * time.Hour,
		},
		{
			name:         "case 3: fail in case new token cannot be created",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			failsCreate:  true,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teleport := New(test.NamespaceName, &config.Config{}, test.NewMockTokenGenerator(test.TokenName))
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				FailsCreate: tc.failsCreate,
			})

			ctx := context.TODO()
			tokenName, err := teleport.GenerateToken(ctx, tc.registerName, tc.tokenType)

			test.CheckError(t, tc.expectError, err)
			if err != nil {
				return
			}

			generatedToken, err := teleport.TeleportClient.GetToken(ctx, tokenName)
			test.CheckError(t, false, err)
			if err == nil {
				expectedExpiryTime := time.Now().Add(tc.expectedExpiry)

				actualToken := generatedToken.(*types.ProvisionTokenV2)
				if !actualToken.GetMetadata().Expires.After(expectedExpiryTime.Add(-time.Minute)) ||
					!actualToken.GetMetadata().Expires.Before(expectedExpiryTime.Add(time.Minute)) {
					t.Fatalf("Expected token expiry to be close to %v, but got %v", expectedExpiryTime, actualToken.GetMetadata().Expires)
				}

				actualRoles := actualToken.GetRoles()
				actualRoleStrings := make([]string, len(actualRoles))
				for i, role := range actualRoles {
					actualRoleStrings[i] = strings.ToLower(role.String())
				}

				if len(tc.expectedRoles) != len(actualRoleStrings) {
					t.Fatalf("Expected roles %v, but got %v", tc.expectedRoles, actualRoleStrings)
				}
				for i := range tc.expectedRoles {
					if tc.expectedRoles[i] != actualRoleStrings[i] {
						t.Fatalf("Expected roles %v, but got %v", tc.expectedRoles, actualRoleStrings)
					}
				}
			}
		})
	}
}

func Test_IsTokenValid(t *testing.T) {
	testCases := []struct {
		name           string
		registerName   string
		tokenName      string
		tokenType      string
		tokens         []types.ProvisionToken
		failsGet       bool
		expectError    bool
		expectedResult bool
	}{
		{
			name:           "case 0: Service should return true for a valid, non-expired token",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:      test.TokenName,
			tokenType:      test.TokenTypeKube,
			tokens:         []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"}, time.Now().Add(1*time.Hour))},
			expectedResult: true,
		},
		{
			name:           "case 1: Service should return false for a non-existent token",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:      test.TokenName,
			tokenType:      test.TokenTypeKube,
			tokens:         nil,
			expectedResult: false,
		},
		{
			name:           "case 2: Service should return false for a token with mismatched type",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:      test.TokenName,
			tokenType:      test.TokenTypeNode,
			tokens:         []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"})},
			expectedResult: false,
		},
		{
			name:         "case 3: client error is surfaced",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:    test.TokenName,
			tokenType:    test.TokenTypeKube,
			failsGet:     true,
			expectError:  true,
		},
		{
			name:           "case 4: Service should return false for an expired token",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:      test.TokenName,
			tokenType:      test.TokenTypeKube,
			tokens:         []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"}, time.Now().Add(-1*time.Hour))},
			expectedResult: false,
		},
		{
			name:           "case 5: Service should return false for a token with mismatched cluster name",
			registerName:   key.GetRegisterName(test.ManagementClusterName, "wrong-cluster"),
			tokenName:      test.TokenName,
			tokenType:      test.TokenTypeKube,
			tokens:         []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"}, time.Now().Add(1*time.Hour))},
			expectedResult: false,
		},
		{
			name:           "case 6: token missing in Teleport is invalid, not an error",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:      "missing",
			tokenType:      test.TokenTypeKube,
			tokens:         nil,
			expectError:    false,
			expectedResult: false,
		},
		{
			name:           "case 7: empty stored token is invalid, not an error",
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:      "",
			tokenType:      test.TokenTypeKube,
			tokens:         nil,
			expectError:    false,
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teleport := New(test.NamespaceName, &config.Config{}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				Tokens:   tc.tokens,
				FailsGet: tc.failsGet,
			})

			ctx := context.TODO()
			isValid, err := teleport.IsTokenValid(ctx, tc.registerName, tc.tokenName, tc.tokenType)

			test.CheckError(t, tc.expectError, err)
			if isValid != tc.expectedResult {
				t.Fatalf("received unexpected result: expected %v, actual %v", tc.expectedResult, isValid)
			}
		})
	}
}

func Test_DeleteTokenByName(t *testing.T) {
	testCases := []struct {
		name        string
		tokenName   string
		tokens      []types.ProvisionToken
		failsDelete bool
		expectError bool
	}{
		{
			name:      "case 0: existing token is removed from Teleport",
			tokenName: test.TokenName,
			tokens:    []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"})},
		},
		{
			name:      "case 1: missing token is not an error",
			tokenName: test.TokenName,
			tokens:    nil,
		},
		{
			name:        "case 2: client error is surfaced",
			tokenName:   test.TokenName,
			tokens:      []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"})},
			failsDelete: true,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			teleport := New(test.NamespaceName, &config.Config{}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				FailsDelete: tc.failsDelete,
				Tokens:      tc.tokens,
			})

			err := teleport.DeleteTokenByName(ctx, log, tc.tokenName)
			test.CheckError(t, tc.expectError, err)
			if err != nil {
				return
			}

			if _, err := teleport.TeleportClient.GetToken(ctx, tc.tokenName); !trace.IsNotFound(err) {
				t.Fatalf("expected token %q to be gone from Teleport, GetToken returned err=%v", tc.tokenName, err)
			}
		})
	}
}

func Test_DeleteClusterTokens(t *testing.T) {
	const kubeTokenName = test.NewTokenName

	testCases := []struct {
		name          string
		secret        *corev1.Secret
		configMap     *corev1.ConfigMap
		tokens        []types.ProvisionToken
		failsDelete   bool
		expectError   bool
		expectAbsent  []string
		expectPresent []string
	}{
		{
			name:      "case 0: node token from Secret and kube token from ConfigMap are both deleted",
			secret:    test.NewSecret(test.ClusterName, test.NamespaceName, test.TokenName),
			configMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, kubeTokenName, []string{"kube"}),
			tokens: []types.ProvisionToken{
				test.NewToken(test.TokenName, test.ClusterName, []string{"node"}),
				test.NewToken(kubeTokenName, test.ClusterName, []string{"kube"}),
			},
			expectAbsent: []string{test.TokenName, kubeTokenName},
		},
		{
			name:      "case 1: only ConfigMap present deletes the kube token and leaves others alone",
			configMap: test.NewConfigMap(test.ClusterName, test.AppName, test.NamespaceName, kubeTokenName, []string{"kube"}),
			tokens: []types.ProvisionToken{
				test.NewToken(test.TokenName, test.ClusterName, []string{"node"}),
				test.NewToken(kubeTokenName, test.ClusterName, []string{"kube"}),
			},
			expectAbsent:  []string{kubeTokenName},
			expectPresent: []string{test.TokenName},
		},
		{
			name: "case 2: neither Secret nor ConfigMap present makes no Teleport call",
			tokens: []types.ProvisionToken{
				test.NewToken(test.TokenName, test.ClusterName, []string{"node"}),
			},
			// Any DeleteToken call would fail, so success proves none was made.
			failsDelete:   true,
			expectPresent: []string{test.TokenName},
		},
		{
			name: "case 3: Secret without a join token is skipped without error or Teleport call",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.GetSecretName(test.ClusterName),
					Namespace: test.NamespaceName,
				},
				Data: map[string][]byte{"unrelated": []byte("value")},
			},
			tokens: []types.ProvisionToken{
				test.NewToken(test.TokenName, test.ClusterName, []string{"node"}),
			},
			failsDelete:   true,
			expectPresent: []string{test.TokenName},
		},
		{
			name: "case 4: ConfigMap without an auth token is skipped without error or Teleport call",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.GetConfigmapName(test.ClusterName, test.AppName),
					Namespace: test.NamespaceName,
				},
				Data: map[string]string{"values": "proxyAddr: 127.0.0.1\n"},
			},
			tokens: []types.ProvisionToken{
				test.NewToken(kubeTokenName, test.ClusterName, []string{"kube"}),
			},
			failsDelete:   true,
			expectPresent: []string{kubeTokenName},
		},
		{
			name:   "case 5: empty stored token names are skipped without error or Teleport call",
			secret: test.NewSecret(test.ClusterName, test.NamespaceName, ""),
			// The dual-block fixture quotes the value, so this stores `authToken: ""`.
			configMap: test.NewDualBlockConfigMap(test.ClusterName, test.AppName, test.NamespaceName, "", []string{"kube"}),
			tokens: []types.ProvisionToken{
				test.NewToken(test.TokenName, test.ClusterName, []string{"node"}),
			},
			failsDelete:   true,
			expectPresent: []string{test.TokenName},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			var objects []client.Object
			if tc.secret != nil {
				objects = append(objects, tc.secret)
			}
			if tc.configMap != nil {
				objects = append(objects, tc.configMap)
			}
			ctrlClient, err := test.NewFakeK8sClientFromObjects(objects...)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			teleport := New(test.NamespaceName, &config.Config{AppName: test.AppName}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				FailsDelete: tc.failsDelete,
				Tokens:      tc.tokens,
			})

			err = teleport.DeleteClusterTokens(ctx, log, ctrlClient, test.ClusterName, test.NamespaceName)
			test.CheckError(t, tc.expectError, err)

			for _, name := range tc.expectAbsent {
				if _, err := teleport.TeleportClient.GetToken(ctx, name); !trace.IsNotFound(err) {
					t.Fatalf("expected token %q to be deleted, GetToken returned err=%v", name, err)
				}
			}
			for _, name := range tc.expectPresent {
				if _, err := teleport.TeleportClient.GetToken(ctx, name); err != nil {
					t.Fatalf("expected token %q to remain, GetToken returned err=%v", name, err)
				}
			}
		})
	}
}
