package teleport

import (
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

type Teleport struct {
	SecretConfig   *SecretConfig
	TeleportClient Client
	Namespace      string
	TokenGenerator token.Generator
}

func New(namespace string, secretConfig *SecretConfig, tokenGenerator token.Generator) *Teleport {
	return &Teleport{
		SecretConfig:   secretConfig,
		Namespace:      namespace,
		TokenGenerator: tokenGenerator,
	}
}
