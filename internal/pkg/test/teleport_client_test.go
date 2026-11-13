package test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
)

func Test_FakeTeleportClient(t *testing.T) {
	testCases := []struct {
		name               string
		config             FakeTeleportClientConfig
		tokenName          string
		storedToken        types.ProvisionToken
		expectedToken      types.ProvisionToken
		expectClientErrors bool
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
				FailsPing:   true,
			},
			expectClientErrors: true,
		},
		{
			name: "case 2: Return expected token",
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
			name: "case 3: Fail to return expected token",
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
			name:               "case 4: Store token",
			config:             FakeTeleportClientConfig{},
			storedToken:        NewToken(TokenName, ClusterName, []string{"kube"}),
			expectClientErrors: false,
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

			token = NewToken(uuid.NewString(), ClusterName, []string{"kube"})
			err = fakeClient.CreateToken(ctx, token)
			CheckError(t, tc.expectClientErrors, err)

			err = fakeClient.DeleteToken(ctx, token.GetName())
			CheckError(t, tc.expectClientErrors, err)

			_, err = fakeClient.Ping(ctx)
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
		})
	}
}

func Test_FakeTeleportClient_DeleteMissingToken(t *testing.T) {
	fakeClient := NewTeleportClient(FakeTeleportClientConfig{})

	err := fakeClient.DeleteToken(context.TODO(), TokenName)
	if !trace.IsNotFound(err) {
		t.Fatalf("expected a NotFound error like the real Teleport client, got %v", err)
	}
}
