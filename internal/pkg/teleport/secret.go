package teleport

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/giantswarm/microerror"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type TeleportSecret struct {
	ProxyAddr             string
	IdentityFile          string
	TeleportVersion       string
	ManagementClusterName string
	AppName               string
	AppVersion            string
	AppCatalog            string
}

func (t *Teleport) GetSecret(ctx context.Context) (*TeleportSecret, error) {
	secret := &corev1.Secret{}

	if err := t.CtrlClient.Get(ctx, types.NamespacedName{
		Name:      key.TeleportOperatorSecretName,
		Namespace: t.Namespace,
	}, secret); err != nil {
		return nil, microerror.Mask(err)
	}

	proxyAddr, err := getSecretString(secret, "proxyAddr")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	identityFile, err := getSecretString(secret, "identityFile")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	managementClusterName, err := getSecretString(secret, "managementClusterName")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	teleportVersion, err := getSecretString(secret, "teleportVersion")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appName, err := getSecretString(secret, "appName")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appVersion, err := getSecretString(secret, "appVersion")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appCatalog, err := getSecretString(secret, "appCatalog")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	return &TeleportSecret{
		IdentityFile:          identityFile,
		ProxyAddr:             proxyAddr,
		ManagementClusterName: managementClusterName,
		TeleportVersion:       teleportVersion,
		AppName:               appName,
		AppVersion:            appVersion,
		AppCatalog:            appCatalog,
	}, nil
}

func (t *Teleport) DeleteSecret(ctx context.Context, cluster *capi.Cluster) error {
	secretName := key.GetSecretName(cluster.Name) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
	}
	t.Logger.Info("Deleting secret...")
	if err := t.CtrlClient.Delete(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			t.Logger.Info("Secret does not exists.")
			return nil
		}
		return microerror.Mask(fmt.Errorf("failed to create Secret: %w", err))
	}
	t.Logger.Info("Secret deleted.")
	return nil
}

func getSecretString(secret *corev1.Secret, key string) (string, error) {
	b, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("malformed Secret: required key %q not found", key)
	}
	return string(b), nil
}

func (t *Teleport) EnsureSecret(ctx context.Context, config *ClusterRegisterConfig) error {
	secretName := key.GetSecretName(config.ClusterName) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: config.InstallNamespace,
		},
	}
	if err := t.CtrlClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: config.InstallNamespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			t.Logger.Info(fmt.Sprintf("Secret does not exist: %s", secretName))
			joinToken, err := t.GenerateJoinToken(ctx, config.RegisterName)
			if err != nil {
				return microerror.Mask(err)
			}
			t.Logger.Info("Generated node join token.")
			secret.StringData = map[string]string{
				"joinToken": joinToken,
			}
			if err := t.CtrlClient.Create(ctx, secret); err != nil {
				return microerror.Mask(fmt.Errorf("failed to create Secret: %w", err))
			} else {
				t.Logger.Info(fmt.Sprintf("Secret created: %s", secretName))
				return nil
			}
		} else {
			return microerror.Mask(fmt.Errorf("failed to get Secret: %w", err))
		}
	}
	t.Logger.Info(fmt.Sprintf("Secret exists: %s", secretName))
	oldTokenBytes, ok := secret.Data["joinToken"]
	if !ok {
		t.Logger.Info("failed to get joinToken from Secret: %s", secretName)
	}
	isTokenValid, err := t.IsTokenValid(ctx, string(oldTokenBytes), config.RegisterName)
	if err != nil {
		return microerror.Mask(fmt.Errorf("failed to verify token validity: %w", err))
	}
	if !isTokenValid {
		t.Logger.Info("Join token has expired.")
		joinToken, err := t.GenerateJoinToken(ctx, config.RegisterName)
		if err != nil {
			return microerror.Mask(err)
		}
		t.Logger.Info("Join token re-generated")
		secret.StringData = map[string]string{
			"joinToken": joinToken,
		}
		if err := t.CtrlClient.Update(ctx, secret); err != nil {
			return microerror.Mask(fmt.Errorf("failed to update Secret: %w", err))
		} else {
			t.Logger.Info(fmt.Sprintf("Secret updated: %s", secretName))
		}
	} else {
		t.Logger.Info("Join token is valid, nothing to do.")
	}
	return nil
}
