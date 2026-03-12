package teleport

import (
	"context"
	"strings"
	"time"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

// IsTokenValid checks whether a Teleport provision token is still usable.
// It fetches the token by name (eliminating the need for the `list` verb on the
// Teleport token resource) and verifies the cluster label, expiry, and roles.
func (t *Teleport) IsTokenValid(ctx context.Context, registerName string, token string, tokenType string) (bool, error) {
	expectedRoles, err := key.ParseRoles(tokenType)
	if err != nil {
		return false, microerror.Mask(err)
	}

	tok, err := t.TeleportClient.GetToken(ctx, token)
	if err != nil {
		if trace.IsNotFound(err) {
			// Token no longer exists in Teleport — treat as invalid, not an error.
			return false, nil
		}
		return false, microerror.Mask(err)
	}

	// Verify the token belongs to the expected cluster (guards against stale Secret
	// references pointing at a token re-used by a different cluster).
	if tok.GetMetadata().Labels["cluster"] != registerName {
		return false, nil
	}

	// Treat the token as invalid when it is within TokenRenewalThreshold of
	// expiry so that agents always receive a token with adequate remaining
	// validity, reducing the risk of a race between rotation and agent restart.
	if tok.Expiry().IsZero() || !tok.Expiry().After(time.Now().Add(key.TokenRenewalThreshold)) {
		return false, nil
	}

	// Check if the token has all the expected roles.
	tokenRoles := tok.GetRoles()
	if len(tokenRoles) != len(expectedRoles) {
		return false, nil
	}
	for _, role := range expectedRoles {
		if !containsRole(tokenRoles, role) {
			return false, nil
		}
	}

	return true, nil
}

func containsRole(roles []types.SystemRole, role string) bool {
	for _, r := range roles {
		if strings.ToLower(r.String()) == role {
			return true
		}
	}
	return false
}

func (t *Teleport) GenerateToken(ctx context.Context, registerName string, roles []string) (string, error) {
	// Short-lived tokens (key.TeleportKubeTokenValidity = 1h) limit the blast
	// radius if a token is stolen from a Kubernetes Secret.
	tokenValidity := time.Now().Add(key.TeleportKubeTokenValidity)
	tokenRoles := key.RolesToSystemRoles(roles)

	token, err := types.NewProvisionToken(t.TokenGenerator.Generate(), tokenRoles, tokenValidity)
	if err != nil {
		return "", microerror.Mask(err)
	}
	// Set cluster label to token
	{
		m := token.GetMetadata()
		m.Labels = map[string]string{
			"cluster": registerName,
			"roles":   key.RolesToString(roles),
		}
		token.SetMetadata(m)
		if err := t.TeleportClient.UpsertToken(ctx, token); err != nil {
			return "", microerror.Mask(err)
		}
	}
	return token.GetName(), nil
}

// DeleteTokenByName deletes a Teleport provision token by its exact name (UUID).
// It is idempotent: if the token has already expired or been deleted it returns
// nil rather than an error. Using the name directly removes the need for the
// `list` verb on the Teleport token resource, shrinking the operator's RBAC
// surface on the Teleport Auth Service.
func (t *Teleport) DeleteTokenByName(ctx context.Context, log logr.Logger, tokenName string) error {
	if err := t.TeleportClient.DeleteToken(ctx, tokenName); err != nil {
		if trace.IsNotFound(err) {
			// Token already expired or was removed externally — nothing to do.
			return nil
		}
		return microerror.Mask(err)
	}
	log.Info("Deleted Teleport join token", "tokenName", tokenName)
	return nil
}
