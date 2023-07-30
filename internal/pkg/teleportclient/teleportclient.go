package teleportclient

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/microerror"
	tc "github.com/gravitational/teleport/api/client"
	tt "github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type TeleportClient struct {
	ProxyAddr             string
	IdentityFile          string
	TeleportVersion       string
	ManagementClusterName string
	AppName               string
	AppVersion            string
	AppCatalog            string
}

const TELEPORT_JOIN_TOKEN_VALIDITY = 24 * time.Hour

func New(namespace string) (*TeleportClient, error) {
	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Println("unable to get config to talk to the apiserver:", err)
		return nil, err
	}

	// Create a new client
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		fmt.Println("unable to create a new client:", err)
		return nil, err
	}

	// Check if the Secret exists
	secret := &corev1.Secret{}
	secretNamespacedName := types.NamespacedName{
		Name:      "teleport-operator",
		Namespace: namespace, // Replace with the correct namespace
	}
	if err := c.Get(context.Background(), secretNamespacedName, secret); err != nil {
		return nil, err
	}

	proxyAddrBytes, proxyAddrOk := secret.Data["proxyAddr"]
	identityFileBytes, identityFileOk := secret.Data["identityFile"]
	managementClusterNameBytes, managementClusterNameOk := secret.Data["managementClusterName"]
	teleportVersionBytes, teleportVersionOk := secret.Data["teleportVersion"]
	appNameBytes, appNameOk := secret.Data["appName"]
	appVersionBytes, appVersionOk := secret.Data["appVersion"]
	appCatalogBytes, appCatalogOk := secret.Data["appCatalog"]
	if !proxyAddrOk && !identityFileOk && !managementClusterNameOk && !teleportVersionOk && appNameOk && appVersionOk && appCatalogOk {
		return nil, fmt.Errorf("malformed Secret: required keys not found")
	}
	identityFile := string(identityFileBytes)
	proxyAddr := string(proxyAddrBytes)
	managementClusterName := string(managementClusterNameBytes)
	teleportVersion := string(teleportVersionBytes)
	appName := string(appNameBytes)
	appVersion := string(appVersionBytes)
	appCatalog := string(appCatalogBytes)

	return &TeleportClient{
		IdentityFile:          identityFile,
		ProxyAddr:             proxyAddr,
		ManagementClusterName: managementClusterName,
		TeleportVersion:       teleportVersion,
		AppName:               appName,
		AppVersion:            appVersion,
		AppCatalog:            appCatalog,
	}, nil
}

func (t *TeleportClient) GetClient(ctx context.Context) (*tc.Client, error) {
	c, err := tc.New(ctx, tc.Config{
		Addrs: []string{
			t.ProxyAddr,
		},
		Credentials: []tc.Credentials{
			tc.LoadIdentityFileFromString(t.IdentityFile),
		},
	})

	if err != nil {
		return nil, microerror.Mask(err)
	}

	_, err = c.Ping(ctx)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	return c, nil
}

func (t *TeleportClient) GetToken(ctx context.Context) (string, error) {
	clt, err := t.GetClient(ctx)
	if err != nil {
		return "", microerror.Mask(err)
	}

	// Look for an existing token or generate one if it's expired
	{
		tokens, err := clt.GetTokens(ctx)
		if err != nil {
			return "", microerror.Mask(err)
		}

		for _, t := range tokens {
			if t.GetMetadata().Labels["operator"] == "teleport-operator" {
				return t.GetName(), nil
			}
		}

		// Generate a token
		expiration := time.Now().Add(TELEPORT_JOIN_TOKEN_VALIDITY)
		token := randSeq(32)
		newToken, err := tt.NewProvisionToken(token, []tt.SystemRole{tt.RoleKube, tt.RoleNode}, expiration)
		if err != nil {
			return "", microerror.Mask(err)
		}
		oldMeta := newToken.GetMetadata()
		oldMeta.Labels = map[string]string{
			"operator": "teleport-operator",
		}
		newToken.SetMetadata(oldMeta)
		err = clt.UpsertToken(ctx, newToken)
		if err != nil {
			return "", microerror.Mask(err)
		}

		return token, nil
	}
}

func (t *TeleportClient) IsTokenValid(ctx context.Context, oldToken string) (bool, error) {
	clt, err := t.GetClient(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}

	{
		tokens, err := clt.GetTokens(ctx)
		if err != nil {
			return false, microerror.Mask(err)
		}

		for _, t := range tokens {
			if t.GetMetadata().Labels["operator"] == "teleport-operator" {
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
	c, err := t.GetClient(ctx)
	if err != nil {
		return false, nil, microerror.Mask(err)
	}

	ks, err := c.GetKubernetesServers(ctx)
	if err != nil {
		return false, nil, microerror.Mask(err)
	}

	for _, k := range ks {
		if k.GetCluster().GetName() == registerName {
			return true, k, nil
		}
	}

	return false, nil, nil
}

func (t *TeleportClient) DeregisterCluster(ctx context.Context, ks tt.KubeServer) error {
	c, err := t.GetClient(ctx)
	if err != nil {
		return microerror.Mask(err)
	}

	if err := c.DeleteKubernetesServer(ctx, ks.GetHostID(), ks.GetCluster().GetName()); err != nil {
		return microerror.Mask(err)
	}

	return nil
}
