package config

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/giantswarm/microerror"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

type IdentityConfig struct {
	IdentityFile string
	LastRead     time.Time
}

func (c *IdentityConfig) Age() float64 {
	now := time.Now()
	diff := now.Sub(c.LastRead)
	return diff.Seconds()
}

func (c *IdentityConfig) Hash() string {
	hasher := sha512.New()
	hasher.Write([]byte(c.IdentityFile))
	sum := hasher.Sum(nil)
	return hex.EncodeToString(sum)
}

func GetIdentityConfigFromSecret(ctx context.Context, ctrlClient client.Client, namespace string) (*IdentityConfig, error) {
	secret := &corev1.Secret{}
	if err := ctrlClient.Get(ctx, types.NamespacedName{
		Name:      key.TeleportBotSecretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, microerror.Mask(err)
	}

	identityFile, err := getSecretString(secret, key.Identity)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	return &IdentityConfig{
		IdentityFile: identityFile,
		LastRead:     time.Now(),
	}, nil
}

func getSecretString(secret *corev1.Secret, key string) (string, error) {
	b, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("malformed Secret: required key %q not found", key)
	}
	return string(b), nil
}
