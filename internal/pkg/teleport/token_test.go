package teleport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gravitational/teleport/api/types"
	ctrl "sigs.k8s.io/controller-runtime"

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
		failsList      bool
		failsDelete    bool
		failsUpsert    bool
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
			name:         "case 3: Fail in case new token cannot be upserted",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			failsUpsert:  true,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teleport := New(test.NamespaceName, &config.Config{}, test.NewMockTokenGenerator(test.TokenName))
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				FailsList:   tc.failsList,
				FailsDelete: tc.failsDelete,
				FailsUpsert: tc.failsUpsert,
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
		failsList      bool
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
			name:         "case 3: Service should fail when token list cannot be retrieved",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			tokenName:    test.TokenName,
			tokenType:    test.TokenTypeKube,
			failsList:    true,
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teleport := New(test.NamespaceName, &config.Config{}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				Tokens:    tc.tokens,
				FailsList: tc.failsList,
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

func Test_DeleteToken(t *testing.T) {
	testCases := []struct {
		name          string
		registerName  string
		token         types.ProvisionToken
		tokens        []types.ProvisionToken
		failsDelete   bool
		expectDeleted bool
		expectError   bool
	}{
		{
			name:          "case 0: Delete token from Teleport",
			registerName:  key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:         test.NewToken(test.TokenName, test.ClusterName, []string{"kube"}),
			tokens:        []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"})},
			expectDeleted: true,
		},
		{
			name:          "case 1: Do not delete token in case cluster label does not match",
			registerName:  test.ManagementClusterName,
			token:         test.NewToken(test.TokenName, test.ClusterName, []string{"kube"}),
			tokens:        []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"})},
			expectDeleted: false,
		},
		{
			name:          "case 2: Succeed in case token does not exist",
			registerName:  key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:         test.NewToken(test.TokenName, test.ManagementClusterName, []string{"kube"}),
			expectDeleted: true,
		},
		{
			name:         "case 3: Fail in case teleport client is unable to delete the token",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			token:        test.NewToken(test.TokenName, test.ManagementClusterName, []string{"kube"}),
			tokens:       []types.ProvisionToken{test.NewToken(test.TokenName, test.ClusterName, []string{"kube"})},
			failsDelete:  true,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var storedToken types.ProvisionToken

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			teleport := New(test.NamespaceName, &config.Config{}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				FailsDelete: tc.failsDelete,
				Tokens:      tc.tokens,
			})

			err := teleport.DeleteToken(ctx, log, tc.registerName)
			test.CheckError(t, tc.expectError, err)
			if err == nil {
				storedToken, err = teleport.TeleportClient.GetToken(ctx, tc.token.GetName())

				if tc.expectDeleted && err == nil {
					t.Fatalf("token %v was not deleted", storedToken)
				}
				if !tc.expectDeleted && err != nil {
					t.Fatalf("token %v was unexpectedly deleted", tc.token)
				}
			}
		})
	}
}
