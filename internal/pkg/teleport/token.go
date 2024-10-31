package teleport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func (t *Teleport) GenerateCIBotToken(ctx context.Context, log logr.Logger, name string) error {
	log.Info("Attempting to generate CI bot token",
		"testClientInitialized", t.TestClient != nil,
		"testInstanceEnabled", t.Config.TestInstance != nil && t.Config.TestInstance.Enabled,
	)

	// Check test instance configuration
	if t.Config.TestInstance == nil || !t.Config.TestInstance.Enabled {
		log.Info("Test instance not configured or not enabled")
		return nil
	}

	// Check test client
	if t.TestClient == nil {
		log.Info("Test client not initialized")
		return nil
	}

	// Generate token
	tokenName := fmt.Sprintf("ci-bot-%s", t.TokenGenerator.Generate())
	token, err := types.NewProvisionToken(
		tokenName,
		[]types.SystemRole{types.RoleBot},
		time.Now().Add(720*time.Hour),
	)
	if err != nil {
		log.Error(err, "Failed to create provision token")
		return err
	}

	// Set metadata
	m := token.GetMetadata()
	m.Labels = map[string]string{
		"type":    "ci-bot",
		"created": time.Now().Format(time.RFC3339),
	}
	token.SetMetadata(m)

	// Create token in test Teleport instance
	if err := t.TestClient.UpsertToken(ctx, token); err != nil {
		return microerror.Mask(err)
	}

	// Store in Kubernetes secret in giantswarm namespace
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "teleport-ci-token",
			Namespace: "giantswarm",
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

	// Create or update secret
	existing := &corev1.Secret{}
	err = t.Client.Get(ctx, client.ObjectKey{
		Name:      secret.Name,
		Namespace: secret.Namespace,
	}, existing)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			if err := t.Client.Create(ctx, secret); err != nil {
				return microerror.Mask(err)
			}
			log.Info("Created CI bot token secret in giantswarm namespace")
			return nil
		}
		return err
	}

	if err := t.Client.Update(ctx, secret); err != nil {
		return microerror.Mask(err)
	}
	log.Info("Updated CI bot token secret in giantswarm namespace")
	return nil
}
