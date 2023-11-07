package teleport

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

type SecretConfig struct {
	ProxyAddr             string
	IdentityFile          string
	TeleportVersion       string
	ManagementClusterName string
	AppName               string
	AppVersion            string
	AppCatalog            string
}

func GetConfigFromSecret(ctx context.Context, ctrlClient client.Client, namespace string) (*SecretConfig, error) {
	secret := &corev1.Secret{}

	if err := ctrlClient.Get(ctx, types.NamespacedName{
		Name:      key.TeleportOperatorSecretName,
		Namespace: namespace,
	}, secret); err != nil {
		return nil, microerror.Mask(err)
	}

	proxyAddr, err := getSecretString(secret, key.ProxyAddr)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	// identityFile, err := getSecretString(secret, key.IdentityFile)
	// if err != nil {
	// 	return nil, microerror.Mask(err)
	// }

	managementClusterName, err := getSecretString(secret, key.ManagementClusterName)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	teleportVersion, err := getSecretString(secret, key.TeleportVersion)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appName, err := getSecretString(secret, key.AppName)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appVersion, err := getSecretString(secret, key.AppVersion)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	appCatalog, err := getSecretString(secret, key.AppCatalog)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	tbotSecret := &corev1.Secret{}

	if err := ctrlClient.Get(ctx, types.NamespacedName{
		Name:      "identity-output",
		Namespace: namespace,
	}, tbotSecret); err != nil {
		return nil, microerror.Mask(err)
	}

	identity, err := getSecretString(tbotSecret, "identity")
	if err != nil {
		return nil, microerror.Mask(err)
	}

	return &SecretConfig{
		IdentityFile:          identity,
		ProxyAddr:             proxyAddr,
		ManagementClusterName: managementClusterName,
		TeleportVersion:       teleportVersion,
		AppName:               appName,
		AppVersion:            appVersion,
		AppCatalog:            appCatalog,
	}, nil
}

func (t *Teleport) GetSecret(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string) (*corev1.Secret, error) {
	var (
		secretName           = key.GetSecretName(clusterName) //#nosec G101
		secretNamespace      = clusterNamespace
		secretNamespacedName = types.NamespacedName{Name: secretName, Namespace: secretNamespace}
		secret               = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		}
	)

	if err := ctrlClient.Get(ctx, secretNamespacedName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, microerror.Mask(fmt.Errorf("failed to get Secret: %w", err))
	}

	return secret, nil
}

func (t *Teleport) GetTokenFromSecret(ctx context.Context, secret *corev1.Secret) (string, error) {
	tokenBytes, ok := secret.Data["joinToken"]
	if !ok {
		return "", microerror.Mask(fmt.Errorf("failed to get joinToken from Secret"))
	}
	return string(tokenBytes), nil
}

func (t *Teleport) CreateSecret(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string, token string) error {
	secretName := key.GetSecretName(clusterName) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: clusterNamespace,
		},
		StringData: map[string]string{
			"joinToken": token,
		},
	}
	if err := ctrlClient.Create(ctx, secret); err != nil {
		return microerror.Mask(fmt.Errorf("failed to create Secret: %w", err))
	}
	log.Info("Created secret with new teleport node join token", "secretName", secretName)
	return nil
}

func (t *Teleport) UpdateSecret(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string, token string) error {
	secretName := key.GetSecretName(clusterName) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: clusterNamespace,
		},
		StringData: map[string]string{
			"joinToken": token,
		},
	}
	if err := ctrlClient.Update(ctx, secret); err != nil {
		return microerror.Mask(fmt.Errorf("failed to update Secret: %w", err))
	}
	log.Info("Updated secret with new teleport node join token", "secretName", secretName)
	return nil
}

func (t *Teleport) DeleteSecret(ctx context.Context, log logr.Logger, ctrlClient client.Client, clusterName string, clusterNamespace string) error {
	secretName := key.GetSecretName(clusterName) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: clusterNamespace,
		},
	}
	if err := ctrlClient.Delete(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return microerror.Mask(fmt.Errorf("failed to delete Secret: %w", err))
	}
	log.Info("Deleted secret", "secretName", secretName)
	return nil
}

func getSecretString(secret *corev1.Secret, key string) (string, error) {
	b, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("malformed Secret: required key %q not found", key)
	}
	return string(b), nil
}
