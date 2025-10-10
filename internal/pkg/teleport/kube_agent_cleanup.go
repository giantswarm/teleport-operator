package teleport

import (
	"context"
	"fmt"
	"strings"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

// DeleteTeleportKubeAgentStateSecrets finds and deletes all teleport-kube-agent state secrets
// in the kube-system namespace. These secrets are created by teleport-kube-agent StatefulSet
// and follow the pattern: teleport-kube-agent-*-state
func (t *Teleport) DeleteTeleportKubeAgentStateSecrets(ctx context.Context, log logr.Logger, ctrlClient client.Client) error {
	// List all secrets in kube-system namespace
	secretList := &corev1.SecretList{}
	listOpts := []client.ListOption{
		client.InNamespace(key.TeleportKubeAppNamespace),
	}

	if err := ctrlClient.List(ctx, secretList, listOpts...); err != nil {
		return microerror.Mask(fmt.Errorf("failed to list secrets in %s namespace: %w", key.TeleportKubeAppNamespace, err))
	}

	deletedCount := 0
	for _, secret := range secretList.Items {
		// Check if the secret name matches the pattern teleport-kube-agent-*-state
		if t.isTeleportKubeAgentStateSecret(secret.Name) {
			if err := ctrlClient.Delete(ctx, &secret); err != nil {
				if apierrors.IsNotFound(err) {
					// Secret was already deleted, continue
					continue
				}
				log.Error(err, "Failed to delete teleport-kube-agent state secret", "secretName", secret.Name)
				return microerror.Mask(fmt.Errorf("failed to delete teleport-kube-agent state secret %s: %w", secret.Name, err))
			}
			log.Info("Deleted teleport-kube-agent state secret", "secretName", secret.Name)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		log.Info("Completed deletion of teleport-kube-agent state secrets", "deletedCount", deletedCount)
	} else {
		log.Info("No teleport-kube-agent state secrets found to delete")
	}

	return nil
}

// RestartTeleportKubeAgentPods finds and deletes all teleport-kube-agent pods
// in the kube-system namespace. This will trigger the StatefulSet to recreate them.
// These pods follow the pattern: teleport-kube-agent-*
func (t *Teleport) RestartTeleportKubeAgentPods(ctx context.Context, log logr.Logger, ctrlClient client.Client) error {
	// List all pods in kube-system namespace
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(key.TeleportKubeAppNamespace),
	}

	if err := ctrlClient.List(ctx, podList, listOpts...); err != nil {
		return microerror.Mask(fmt.Errorf("failed to list pods in %s namespace: %w", key.TeleportKubeAppNamespace, err))
	}

	deletedCount := 0
	for _, pod := range podList.Items {
		// Check if the pod name matches the pattern teleport-kube-agent-*
		if t.isTeleportKubeAgentPod(pod.Name) {
			if err := ctrlClient.Delete(ctx, &pod); err != nil {
				if apierrors.IsNotFound(err) {
					// Pod was already deleted, continue
					continue
				}
				log.Error(err, "Failed to delete teleport-kube-agent pod", "podName", pod.Name)
				return microerror.Mask(fmt.Errorf("failed to delete teleport-kube-agent pod %s: %w", pod.Name, err))
			}
			log.Info("Deleted teleport-kube-agent pod (will be recreated by StatefulSet)", "podName", pod.Name)
			deletedCount++
		}
	}

	if deletedCount > 0 {
		log.Info("Completed restart of teleport-kube-agent pods", "deletedCount", deletedCount)
	} else {
		log.Info("No teleport-kube-agent pods found to restart")
	}

	return nil
}

// isTeleportKubeAgentStateSecret checks if a secret name matches the pattern teleport-kube-agent-*-state
func (t *Teleport) isTeleportKubeAgentStateSecret(secretName string) bool {
	return strings.HasPrefix(secretName, "teleport-kube-agent-") && strings.HasSuffix(secretName, "-state")
}

// isTeleportKubeAgentPod checks if a pod name matches the pattern teleport-kube-agent-N
// where N is a number (StatefulSet pod pattern)
func (t *Teleport) isTeleportKubeAgentPod(podName string) bool {
	if !strings.HasPrefix(podName, "teleport-kube-agent-") {
		return false
	}

	// Extract the suffix after "teleport-kube-agent-"
	suffix := strings.TrimPrefix(podName, "teleport-kube-agent-")

	// Check if the suffix is just a number (StatefulSet pod pattern)
	// This will match "0", "1", "10", etc. but not "0-state", "0-config", etc.
	for _, char := range suffix {
		if char < '0' || char > '9' {
			return false
		}
	}

	// Must have at least one digit
	return len(suffix) > 0
}
