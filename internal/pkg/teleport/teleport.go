package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

type Teleport struct {
	Config         *config.Config
	PrimaryClient  Client
	TestClient     Client
	Identity       *config.IdentityConfig
	TeleportClient Client
	Namespace      string
	TokenGenerator token.Generator
	Client         client.Client
	Log            logr.Logger
}

func New(namespace string, cfg *config.Config, tokenGenerator token.Generator) *Teleport {
	return &Teleport{
		Config:         cfg,
		Namespace:      namespace,
		TokenGenerator: tokenGenerator,
		Log:            logr.Discard(),
	}
}

func (t *Teleport) WithLog(log logr.Logger) *Teleport {
	t.Log = log
	return t
}

func (t *Teleport) InitializeClients(ctx context.Context) error {
	var err error

	// Initialize primary client first
	primaryIdentity, err := config.GetIdentityConfigFromSecret(ctx, t.Client, t.Namespace)
	if err != nil {
		return microerror.Mask(err)
	}

	t.PrimaryClient, err = NewClient(ctx, t.Config.ProxyAddr, primaryIdentity.IdentityFile)
	if err != nil {
		return microerror.Mask(err)
	}

	// Initialize test client if configured - but don't fail if it can't connect
	if t.Config.TestInstance != nil && t.Config.TestInstance.Enabled {
		t.Log.Info("Attempting to initialize test client",
			"proxyAddr", t.Config.TestInstance.ProxyAddr)

		testIdentity, err := config.GetIdentityConfigFromSecret(ctx, t.Client, t.Namespace)
		if err != nil {
			t.Log.Info("Failed to get test instance identity", "error", err)
			return nil
		}

		t.TestClient, err = NewClient(ctx, t.Config.TestInstance.ProxyAddr, testIdentity.IdentityFile)
		if err != nil {
			t.Log.Info("Failed to initialize test client",
				"error", err,
				"proxyAddr", t.Config.TestInstance.ProxyAddr)
			return nil // Don't fail the operator if test client fails
		}

		t.Log.Info("Successfully initialized test client")
	}

	return nil
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
