package test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/teleport/api/types"
)

func Test_FakeTeleportClient(t *testing.T) {
	testCases := []struct {
		name               string
		config             FakeTeleportClientConfig
		tokenName          string
		storedToken        types.ProvisionToken
		upsertedToken      types.ProvisionToken
		expectedTokens     []types.ProvisionToken
		expectedToken      types.ProvisionToken
		expectClientErrors bool
		expectTokensError  bool
		expectTokenError   bool
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
					NewToken(TokenName, ClusterName, []string{"kube"}),
					NewToken(NewTokenName, ClusterName, []string{"kube"}),
				},
			},
			expectedTokens: []types.ProvisionToken{
				NewToken(TokenName, ClusterName, []string{"kube"}),
				NewToken(NewTokenName, ClusterName, []string{"kube"}),
			},
			expectClientErrors: false,
		},
		{
			name: "case 3, Fail to return list of tokens",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, []string{"kube"}),
					NewToken(NewTokenName, ClusterName, []string{"kube"}),
				},
			},
			expectClientErrors: false,
			expectTokensError:  true,
		},
		{
			name: "case 4: Return expected token",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, []string{"kube"}),
				},
			},
			tokenName:          TokenName,
			expectedToken:      NewToken(TokenName, ClusterName, []string{"kube"}),
			expectClientErrors: false,
		},
		{
			name: "case 5: Fail to return expected token",
			config: FakeTeleportClientConfig{
				Tokens: []types.ProvisionToken{
					NewToken(TokenName, ClusterName, []string{"kube"}),
				},
			},
			tokenName:          NewTokenName,
			expectClientErrors: false,
			expectTokenError:   true,
		},
		{
			name:               "case 6: Store token",
			config:             FakeTeleportClientConfig{},
			storedToken:        NewToken(TokenName, ClusterName, []string{"kube"}),
			expectClientErrors: false,
			expectTokensError:  false,
		},
		{
			name:               "case 7: Upsert token",
			config:             FakeTeleportClientConfig{},
			upsertedToken:      NewToken(TokenName, ClusterName, []string{"node"}),
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

			if len(tc.expectedTokens) > 0 {
				var tokens []types.ProvisionToken
				tokens, err = fakeClient.GetTokens(ctx)
				CheckError(t, tc.expectTokensError, err)
				if err == nil {
					CheckTokens(t, tc.expectedTokens, tokens)
				}
			}

			token = NewToken(uuid.NewString(), ClusterName, []string{"kube"})
			err = fakeClient.CreateToken(ctx, token)
			CheckError(t, tc.expectClientErrors, err)

			err = fakeClient.DeleteToken(ctx, token.GetName())
			CheckError(t, tc.expectClientErrors, err)

			_, err = fakeClient.Ping(ctx)
			CheckError(t, tc.expectClientErrors, err)

			err = fakeClient.UpsertToken(ctx, token)
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
