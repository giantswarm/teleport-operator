package teleport

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, registerName string, token string, tokenType string) (bool, error) {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}

	expectedRoles, err := key.ParseRoles(tokenType)
	if err != nil {
		return false, microerror.Mask(err)
	}

	for _, t := range tokens {
		if t.GetName() == token &&
			t.GetMetadata().Labels["cluster"] == registerName {
			// Check if the token has expired
			if !t.Expiry().IsZero() && t.Expiry().After(time.Now()) {
				// Check if the token has all the expected roles
				tokenRoles := t.GetRoles()
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
			return false, nil
		}
	}

	// If we didn't find a matching token, it's not valid
	return false, nil
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
		if err := t.TeleportClient.UpsertToken(ctx, token); err != nil {
			return "", microerror.Mask(err)
		}
	}
	return token.GetName(), nil
}

func (t *Teleport) DeleteToken(ctx context.Context, log logr.Logger, registerName string) error {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if token.GetMetadata().Labels["cluster"] == registerName {
			if err := t.TeleportClient.DeleteToken(ctx, token.GetName()); err != nil {
				return microerror.Mask(err)
			}
			log.Info("Deleted teleport node/kube join token for the cluster", "registerName", registerName)
			return nil
		}
	}
	return nil
}

func (t *Teleport) GenerateCIBotToken(ctx context.Context, log logr.Logger, registerName string) error {
	// First check if test instance is configured
	if t.Config.TestInstance == nil || !t.Config.TestInstance.Enabled {
		return nil
	}

	// Check if TestClient is initialized
	if t.TestClient == nil {
		return nil
	}

	tokens, err := t.TestClient.GetTokens(ctx)
	if err != nil {
		return err
	}

	// Check for existing token
	for _, token := range tokens {
		if token.GetMetadata().Labels["type"] == "ci-bot" &&
			token.GetMetadata().Labels["cluster"] == registerName {
			if !token.Expiry().IsZero() && token.Expiry().After(time.Now()) {
				log.Info("Found valid existing CI bot token", "registerName", registerName)
				return nil
			}
		}
	}

	// Generate new token
	tokenValidity := time.Now().Add(key.TeleportKubeTokenValidity)
	tokenRoles := []types.SystemRole{types.RoleBot}

	token, err := types.NewProvisionToken(t.TokenGenerator.Generate(), tokenRoles, tokenValidity)
	if err != nil {
		return microerror.Mask(err)
	}

	// Set metadata
	m := token.GetMetadata()
	m.Labels = map[string]string{
		"type":    "ci-bot",
		"cluster": registerName,
		"roles":   "bot",
	}
	token.SetMetadata(m)

	if err := t.TestClient.UpsertToken(ctx, token); err != nil {
		return microerror.Mask(err)
	}

	// Create or patch K8s secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "teleport-ci-token",
			Namespace: "mc-bootstrap",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "teleport-operator",
				"app.kubernetes.io/component":  "ci-bot",
				"app.kubernetes.io/managed-by": "teleport-operator",
			},
		},
		StringData: map[string]string{
			"token": token.GetName(),
			"proxy": t.Config.TestInstance.ProxyAddr,
		},
	}

	// Get existing secret if it exists
	existing := &corev1.Secret{}
	err = t.Client.Get(ctx, client.ObjectKey{
		Namespace: secret.Namespace,
		Name:      secret.Name,
	}, existing)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			if err := t.Client.Create(ctx, secret); err != nil {
				return microerror.Mask(err)
			}
			log.Info("Created CI bot token secret", "registerName", registerName)
			return nil
		}
		return microerror.Mask(err)
	}

	// Create a patch helper
	patchHelper, err := patch.NewHelper(existing, t.Client)
	if err != nil {
		return microerror.Mask(err)
	}

	// Update the secret data
	existing.StringData = secret.StringData
	existing.Labels = secret.Labels

	// Apply the patch
	if err := patchHelper.Patch(ctx, existing); err != nil {
		return microerror.Mask(err)
	}

	log.Info("Updated CI bot token secret", "registerName", registerName)
	return nil
}
