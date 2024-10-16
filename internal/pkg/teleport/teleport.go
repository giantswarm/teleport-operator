package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

type Teleport struct {
	Config         *config.Config
	Identity       *config.IdentityConfig
	TeleportClient Client
	Namespace      string
	TokenGenerator token.Generator
	Client         client.Client
}

func New(namespace string, cfg *config.Config, tokenGenerator token.Generator) *Teleport {
	return &Teleport{
		Config:         cfg,
		Namespace:      namespace,
		TokenGenerator: tokenGenerator,
	}
}

func (t *Teleport) AreTeleportAppsEnabled(ctx context.Context, clusterName, namespace string) (bool, error) {
	configMap := &corev1.ConfigMap{}
	err := t.Client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-teleport-kube-agent-user-values", clusterName),
		Namespace: namespace,
	}, configMap)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return false, microerror.Mask(err)
		}
		return false, nil // ConfigMap not found, return false without error
	}

	valuesYaml, ok := configMap.Data["values"]
	if !ok {
		return false, nil // No values key, apps are not enabled
	}

	var values map[string]interface{}
	err = yaml.Unmarshal([]byte(valuesYaml), &values)
	if err != nil {
		return false, microerror.Mask(err)
	}

	apps, ok := values["apps"].([]interface{})
	return ok && len(apps) > 0, nil
}
