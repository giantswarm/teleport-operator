package teleport

import (
	tc "github.com/gravitational/teleport/api/client"
)

type Teleport struct {
	SecretConfig   *SecretConfig
	TeleportClient *tc.Client
	Namespace      string
}

func New(namespace string, secretConfig *SecretConfig) *Teleport {
	return &Teleport{
		SecretConfig: secretConfig,
		Namespace:    namespace,
	}
}
