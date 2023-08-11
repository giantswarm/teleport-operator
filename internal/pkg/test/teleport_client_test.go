package test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/teleport/api/types"
)

const (
	hostId  = "test-host-id"
	host1Id = "test-host-1-id"
	host2Id = "test-host-2-id"

	hostName  = "test-host-name"
	host1Name = "test-host-1-name"
	host2Name = "test-host-2-name"
)

func Test_FakeTeleportClient(t *testing.T) {
	testCases := []struct {
		name                  string
		config                FakeTeleportClientConfig
		tokenName             string
		storedToken           types.ProvisionToken
		upsertedToken         types.ProvisionToken
		expectedTokens        []types.ProvisionToken
		expectedToken         types.ProvisionToken
		expectedK8sServers    []types.KubeServer
		expectClientErrors    bool
		expectTokensError     bool
		expectTokenError      bool
		expectK8sServersError bool
	}{
		{
			name:               "case 0: Return valid client and perform all tasks successfully",
			expectClientErrors: false,
		},
		{
			name: "case 1: Return valid client and fail to perform tasks",
			config: FakeTeleportClientConfig{
				FailsCreate: true,
				FailsDelete: true,
				FailsGet:    true,
				FailsList:   true,
				FailsPing:   true,
				FailsUpsert: true,
			},
			expectClientErrors: true,
		},
		{
			name: "case 2: Return expected list of tokens",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, TokenTypeKube),
					NewToken(NewTokenName, ClusterName, TokenTypeKube),
				},
			},
			expectedTokens: []types.ProvisionToken{
				NewToken(TokenName, ClusterName, TokenTypeKube),
				NewToken(NewTokenName, ClusterName, TokenTypeKube),
			},
			expectClientErrors: false,
		},
		{
			name: "case 3, Fail to return list of tokens",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, TokenTypeKube),
					NewToken(NewTokenName, ClusterName, TokenTypeKube),
				},
			},
			expectClientErrors: false,
			expectTokensError:  true,
		},
		{
			name: "case 4: Return expected token",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, TokenTypeKube),
				},
			},
			tokenName:          TokenName,
			expectedToken:      NewToken(TokenName, ClusterName, TokenTypeKube),
			expectClientErrors: false,
		},
		{
			name: "case 5: Fail to return expected token",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, TokenTypeKube),
				},
			},
			tokenName:          NewTokenName,
			expectClientErrors: false,
			expectTokenError:   true,
		},
		{
			name: "case 6: Return expected list of Kubernetes servers",
			config: FakeTeleportClientConfig{
				KubernetesServers: []types.KubeServer{
					NewKubeServer(ClusterName, host1Id, host1Name),
					NewKubeServer(ManagementClusterName, host2Id, host2Name),
				},
			},
			expectedK8sServers: []types.KubeServer{
				NewKubeServer(ClusterName, host1Id, host1Name),
				NewKubeServer(ManagementClusterName, host2Id, host2Name),
			},
			expectClientErrors: false,
		},
		{
			name: "case 7: Fail to return list of Kubernetes servers",
			config: FakeTeleportClientConfig{
				KubernetesServers: []types.KubeServer{
					NewKubeServer(ClusterName, host1Id, host1Name),
					NewKubeServer(ManagementClusterName, host2Id, host2Name),
				},
			},
			expectClientErrors:    false,
			expectTokensError:     true,
			expectK8sServersError: true,
		},
		{
			name:               "case 8: Store token",
			config:             FakeTeleportClientConfig{},
			storedToken:        NewToken(TokenName, ClusterName, TokenTypeKube),
			expectClientErrors: false,
			expectTokensError:  false,
		},
		{
			name:               "case 9: Upsert token",
			config:             FakeTeleportClientConfig{},
			upsertedToken:      NewToken(TokenName, ClusterName, TokenTypeNode),
			expectClientErrors: false,
			expectTokensError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				token types.ProvisionToken
				err   error
			)

			ctx := context.TODO()

			fakeClient := NewTeleportClient(tc.config)

			if tc.tokenName != "" {
				token, err = fakeClient.GetToken(ctx, tc.tokenName)
				CheckError(t, tc.expectTokenError, err)
				if err == nil {
					CheckToken(t, tc.expectedToken, token)
				}
			}

			if tc.expectedTokens != nil && len(tc.expectedTokens) > 0 {
				var tokens []types.ProvisionToken
				tokens, err = fakeClient.GetTokens(ctx)
				CheckError(t, tc.expectTokensError, err)
				if err == nil {
					CheckTokens(t, tc.expectedTokens, tokens)
				}
			}

			if tc.expectedK8sServers != nil && len(tc.expectedK8sServers) > 0 {
				var servers []types.KubeServer
				servers, err = fakeClient.GetKubernetesServers(ctx)
				CheckError(t, tc.expectK8sServersError, err)
				if err == nil {
					CheckK8sServers(t, tc.expectedK8sServers, servers)
				}
			}

			token = NewToken(uuid.NewString(), ClusterName, TokenTypeKube)
			err = fakeClient.CreateToken(ctx, token)
			CheckError(t, tc.expectClientErrors, err)

			err = fakeClient.DeleteToken(ctx, token.GetName())
			CheckError(t, tc.expectClientErrors, err)

			_, err = fakeClient.Ping(ctx)
			CheckError(t, tc.expectClientErrors, err)

			err = fakeClient.UpsertToken(ctx, token)
			CheckError(t, tc.expectClientErrors, err)

			err = fakeClient.DeleteKubernetesServer(ctx, hostId, hostName)
			CheckError(t, tc.expectClientErrors, err)

			if tc.storedToken != nil {
				err = fakeClient.CreateToken(ctx, tc.storedToken)
				CheckError(t, tc.expectTokenError, err)

				storedToken, err := fakeClient.GetToken(ctx, tc.storedToken.GetName())
				CheckError(t, tc.expectTokenError, err)

				if err == nil {
					CheckToken(t, tc.storedToken, storedToken)
				}
			}

			if tc.upsertedToken != nil {
				err = fakeClient.UpsertToken(ctx, tc.upsertedToken)
				CheckError(t, tc.expectTokenError, err)

				err = fakeClient.UpsertToken(ctx, tc.upsertedToken)
				CheckError(t, tc.expectTokenError, err)

				upsertedToken, err := fakeClient.GetToken(ctx, tc.upsertedToken.GetName())
				CheckError(t, tc.expectTokenError, err)

				if err == nil {
					CheckToken(t, tc.upsertedToken, upsertedToken)
				}
			}
		})
	}
}
