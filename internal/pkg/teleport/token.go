package teleport

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, registerName string, token string, tokenType string) (bool, error) {
	// Teleport rejects an empty name with BadParameter; an empty stored token
	// is simply invalid and gets rotated.
	if token == "" {
		return false, nil
	}

	provisionToken, err := t.TeleportClient.GetToken(ctx, token)
	if trace.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, microerror.Mask(err)
	}

	expectedRoles, err := key.ParseRoles(tokenType)
	if err != nil {
		return false, microerror.Mask(err)
	}

	if provisionToken.GetMetadata().Labels["cluster"] != registerName {
		return false, nil
	}

	// Check if the token has expired
	if provisionToken.Expiry().IsZero() || !provisionToken.Expiry().After(time.Now()) {
		return false, nil
	}

	// Check if the token has all the expected roles
	tokenRoles := provisionToken.GetRoles()
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
		if err := t.TeleportClient.CreateToken(ctx, token); err != nil {
			return "", microerror.Mask(err)
		}
	}
	return token.GetName(), nil
}

// errEmptyStoredToken marks a Secret or ConfigMap whose stored token is empty.
// Teleport rejects an empty token name, so such a token is skipped like an
// unreadable one.
var errEmptyStoredToken = errors.New("stored join token is empty")

// DeleteTokenByName deletes the Teleport join token with the given name. A
// token that no longer exists is treated as already deleted. The name is the
// join secret itself, so it is never logged.
func (t *Teleport) DeleteTokenByName(ctx context.Context, log logr.Logger, name string) error {
	if err := t.TeleportClient.DeleteToken(ctx, name); err != nil {
		if trace.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(err)
	}
	log.Info("Deleted teleport join token")
	return nil
}

// DeleteClusterTokens deletes the cluster's node join token, stored in its
// Secret, and its kube/app join token, stored in its ConfigMap. A missing
// Secret or ConfigMap means there is no stored token to delete. One that holds
// no readable token is logged and skipped, so cluster deletion never gets stuck
// on a malformed object; a token left behind that way expires on its own.
func (t *Teleport) DeleteClusterTokens(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName, clusterNamespace string) error {
	secret, err := t.GetSecret(ctx, log, ctrlClient, clusterName, clusterNamespace)
	if err != nil {
		return microerror.Mask(err)
	}
	if secret != nil {
		nodeToken, err := t.GetTokenFromSecret(ctx, secret)
		if err == nil && nodeToken == "" {
			err = errEmptyStoredToken
		}
		if err != nil {
			log.Error(err, "Skipping teleport join token deletion: no readable token", "kind", "Secret", "name", secret.GetName())
		} else if err := t.DeleteTokenByName(ctx, log, nodeToken); err != nil {
			return microerror.Mask(err)
		}
	}

	configMap, err := t.GetConfigMap(ctx, log, ctrlClient, clusterName, clusterNamespace)
	if err != nil {
		return microerror.Mask(err)
	}
	if configMap != nil {
		kubeToken, err := t.GetTokenFromConfigMap(ctx, configMap)
		if err == nil && kubeToken == "" {
			err = errEmptyStoredToken
		}
		if err != nil {
			log.Error(err, "Skipping teleport join token deletion: no readable token", "kind", "ConfigMap", "name", configMap.GetName())
		} else if err := t.DeleteTokenByName(ctx, log, kubeToken); err != nil {
			return microerror.Mask(err)
		}
	}

	return nil
}
