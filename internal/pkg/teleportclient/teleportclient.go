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
	ManagementClusterName string
}

const TELEPORT_JOIN_TOKEN_VALIDITY = 1 * time.Hour

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
	if !proxyAddrOk && !identityFileOk && !managementClusterNameOk {
		return nil, fmt.Errorf("malformed Secret: `identityFile` or `proxyAddr` or `managementClusterName` key not found")
	}
	identityFile := string(identityFileBytes)
	proxyAddr := string(proxyAddrBytes)
	managementClusterName := string(managementClusterNameBytes)

	return &TeleportClient{
		IdentityFile:          identityFile,
		ProxyAddr:             proxyAddr,
		ManagementClusterName: managementClusterName,
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

func (t *TeleportClient) GetToken(ctx context.Context, clusterName string) (string, error) {
	clt, err := t.GetClient(ctx)
	if err != nil {
		return "", microerror.Mask(err)
	}

	// Look for an existing token or generate one
	{
		tokens, err := clt.GetTokens(ctx)
		if err != nil {
			return "", microerror.Mask(err)
		}

		for _, t := range tokens {
			if t.GetMetadata().Labels["cluster"] == clusterName {
				err = clt.DeleteToken(ctx, t.GetName())
				if err != nil {
					return "", microerror.Mask(err)
				}
				break
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
			"cluster": clusterName,
		}
		newToken.SetMetadata(oldMeta)
		err = clt.UpsertToken(ctx, newToken)
		if err != nil {
			return "", microerror.Mask(err)
		}

		return token, nil
	}
}

func (t *TeleportClient) HasTokenExpired(ctx context.Context, clusterName string) (bool, error) {
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
			if t.GetMetadata().Labels["cluster"] == clusterName {
				// Nearing expiry, less than an hour, refresh it
				// if time.Since(*t.GetMetadata().Expires).Hours() < -1 {
				// 	return true, nil
				// }
				return false, nil
			}
		}
		return true, nil
	}
}

func (t *TeleportClient) ClusterExists(ctx context.Context, clusterName string) (bool, error) {
	c, err := t.GetClient(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}

	cluster, err := c.GetKubernetesCluster(ctx, clusterName)
	if err != nil {
		return false, microerror.Mask(err)
	}

	if cluster.GetName() == clusterName {
		return true, nil
	}

	return false, nil
}
