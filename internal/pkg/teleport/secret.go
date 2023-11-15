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
