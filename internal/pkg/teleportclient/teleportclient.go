package teleportclient

import (
	"context"
	"fmt"
	"time"

	tc "github.com/gravitational/teleport/api/client"
	tt "github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

type TeleportClient struct {
	ProxyAddr             string
	IdentityFile          string
	TeleportVersion       string
	ManagementClusterName string
	AppName               string
	AppVersion            string
	AppCatalog            string
	Client                *tc.Client
}

func New(namespace string) (*TeleportClient, error) {
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get config to talk to the apiserver: %s", err)
	}

	// Create a new client
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("unable to create a new client: %s", err)
	}

	// Check if the Secret exists
	secret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      key.TeleportOperatorSecretName,
		Namespace: namespace, // Replace with the correct namespace
	}
	if err := c.Get(context.Background(), secretNamespacedName, secret); err != nil {
		return nil, err
	}

	proxyAddr, err := getSecretString(secret, "proxyAddr")
	if err != nil {
		return nil, err
	}

	identityFile, err := getSecretString(secret, "identityFile")
	if err != nil {
		return nil, err
	}

	managementClusterName, err := getSecretString(secret, "managementClusterName")
	if err != nil {
		return nil, err
	}

	teleportVersion, err := getSecretString(secret, "teleportVersion")
	if err != nil {
		return nil, err
	}

	appName, err := getSecretString(secret, "appName")
	if err != nil {
		return nil, err
	}

	appVersion, err := getSecretString(secret, "appVersion")
	if err != nil {
		return nil, err
	}

	appCatalog, err := getSecretString(secret, "appCatalog")
	if err != nil {
		return nil, err
	}

	client, err := getClient(context.TODO(), proxyAddr, identityFile)
	if err != nil {
		return nil, err
	}

	return &TeleportClient{
		IdentityFile:          identityFile,
		ProxyAddr:             proxyAddr,
		ManagementClusterName: managementClusterName,
		TeleportVersion:       teleportVersion,
		AppName:               appName,
		AppVersion:            appVersion,
		AppCatalog:            appCatalog,
		Client:                client,
	}, nil
}

func getClient(ctx context.Context, proxyAddr, identityFile string) (*tc.Client, error) {
	c, err := tc.New(ctx, tc.Config{
		Addrs: []string{
			proxyAddr,
		},
		Credentials: []tc.Credentials{
			tc.LoadIdentityFileFromString(identityFile),
		},
	})

	if err != nil {
		return nil, err
	}

	_, err = c.Ping(ctx)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (t *TeleportClient) GetToken(ctx context.Context, registerName string) (string, error) {
	// Look for an existing token or generate one if it's expired
	tokens, err := t.Client.GetTokens(ctx)
	if err != nil {
		return "", err
	}

	for _, t := range tokens {
		if t.GetMetadata().Labels["cluster"] == registerName {
			return t.GetName(), nil
		}
	}

	// Generate a token
	expiration := time.Now().Add(key.TeleportJoinTokenValidity)
	token := randSeq(32)
	newToken, err := tt.NewProvisionToken(token, []tt.SystemRole{tt.RoleKube, tt.RoleNode}, expiration)
	if err != nil {
		return "", err
	}
	metadata := newToken.GetMetadata()
	metadata.Labels = map[string]string{
		"cluster": registerName,
	}
	newToken.SetMetadata(metadata)
	err = t.Client.UpsertToken(ctx, newToken)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (t *TeleportClient) IsTokenValid(ctx context.Context, oldToken string, registerName string) (bool, error) {
	{
		tokens, err := t.Client.GetTokens(ctx)
		if err != nil {
			return false, err
		}

		for _, t := range tokens {
			if t.GetMetadata().Labels["cluster"] == registerName {
				if t.GetName() == oldToken {
					return true, nil
				}
				return false, nil
			}
		}
		return false, nil
	}
}

func (t *TeleportClient) IsClusterRegistered(ctx context.Context, registerName string) (bool, tt.KubeServer, error) {
	ks, err := t.Client.GetKubernetesServers(ctx)
	if err != nil {
		return false, nil, err
	}

	for _, k := range ks {
		if k.GetCluster().GetName() == registerName {
			return true, k, nil
		}
	}

	return false, nil, nil
}

func (t *TeleportClient) DeregisterCluster(ctx context.Context, ks tt.KubeServer) error {
	if err := t.Client.DeleteKubernetesServer(ctx, ks.GetHostID(), ks.GetCluster().GetName()); err != nil {
		return err
	}

	return nil
}

func getSecretString(secret *corev1.Secret, key string) (string, error) {
	b, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("malformed Secret: required key %q not found", key)
	}
	return string(b), nil
}
