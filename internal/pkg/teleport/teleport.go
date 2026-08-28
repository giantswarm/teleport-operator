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
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
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

	// The apps may sit flat at the root, or nested under the
	// `teleport-kube-agent` key that chart v0.11.0+ reads (see
	// key.UsesNestedKubeAgentValues). Accept either, and don't let one layout
	// mask the other: during the migration installations legitimately carry
	// both, and a nested block that exists for some unrelated setting must not
	// hide a flat `apps` list. Getting this wrong drops the `app` role, which
	// silently stops advertising every app instead of failing loudly.
	if hasApps(values) {
		return true, nil
	}
	if nested, ok := values[key.TeleportKubeAgentValuesKey].(map[string]interface{}); ok {
		return hasApps(nested), nil
	}

	return false, nil
}

// hasApps reports whether the given values define a non-empty `apps` list.
func hasApps(values map[string]interface{}) bool {
	apps, ok := values["apps"].([]interface{})
	return ok && len(apps) > 0
}
