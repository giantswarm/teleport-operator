package teleport

import (
	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

type Teleport struct {
	Config         *config.Config
	Identity       *config.IdentityConfig
	TeleportClient Client
	Namespace      string
	TokenGenerator token.Generator
}

func New(namespace string, cfg *config.Config, tokenGenerator token.Generator) *Teleport {
	return &Teleport{
		Config:         cfg,
		Namespace:      namespace,
		TokenGenerator: tokenGenerator,
	}
}
