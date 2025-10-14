package teleport

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func TestDeleteTeleportKubeAgentStateSecrets(t *testing.T) {
	tests := []struct {
		name            string
		existingSecrets []corev1.Secret
		expectedDeleted int
	}{
		{
			name: "Delete teleport-kube-agent state secrets",
			existingSecrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-0-state",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-1-state",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-2-state",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-secret",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-0-config", // Should not be deleted
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
			},
			expectedDeleted: 3,
		},
		{
			name:            "No state secrets to delete",
			existingSecrets: []corev1.Secret{},
			expectedDeleted: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client with existing secrets
			scheme := runtime.NewScheme()
			corev1.AddToScheme(scheme)

			objects := make([]client.Object, len(tt.existingSecrets))
			for i := range tt.existingSecrets {
				objects[i] = &tt.existingSecrets[i]
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create teleport instance
			cfg := &config.Config{}
			teleportClient := New("test-namespace", cfg, token.NewGenerator())

			log := logr.Discard()

			// Call the function
			err := teleportClient.DeleteTeleportKubeAgentStateSecrets(context.Background(), log, fakeClient)
			if err != nil {
				t.Fatalf("DeleteTeleportKubeAgentStateSecrets() error = %v", err)
			}

			// Verify secrets were deleted
			secretList := &corev1.SecretList{}
			err = fakeClient.List(context.Background(), secretList, client.InNamespace(key.TeleportKubeAppNamespace))
			if err != nil {
				t.Fatalf("Failed to list secrets: %v", err)
			}

			remainingStateSecrets := 0
			for _, secret := range secretList.Items {
				if teleportClient.isTeleportKubeAgentStateSecret(secret.Name) {
					remainingStateSecrets++
				}
			}

			if remainingStateSecrets != 0 {
				t.Errorf("Expected 0 remaining state secrets, got %d", remainingStateSecrets)
			}

			// Count total remaining secrets (should be original count minus expected deleted)
			expectedRemaining := len(tt.existingSecrets) - tt.expectedDeleted
			if len(secretList.Items) != expectedRemaining {
				t.Errorf("Expected %d remaining secrets, got %d", expectedRemaining, len(secretList.Items))
			}
		})
	}
}

func TestRestartTeleportKubeAgentPods(t *testing.T) {
	tests := []struct {
		name            string
		existingPods    []corev1.Pod
		expectedDeleted int
	}{
		{
			name: "Delete teleport-kube-agent pods",
			existingPods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-0",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-1",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-2",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-pod",
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "teleport-kube-agent-0-state", // Should not be deleted (invalid pod name)
						Namespace: key.TeleportKubeAppNamespace,
					},
				},
			},
			expectedDeleted: 3,
		},
		{
			name:            "No agent pods to delete",
			existingPods:    []corev1.Pod{},
			expectedDeleted: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client with existing pods
			scheme := runtime.NewScheme()
			corev1.AddToScheme(scheme)

			objects := make([]client.Object, len(tt.existingPods))
			for i := range tt.existingPods {
				objects[i] = &tt.existingPods[i]
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create teleport instance
			cfg := &config.Config{}
			teleportClient := New("test-namespace", cfg, token.NewGenerator())

			log := logr.Discard()

			// Call the function
			err := teleportClient.RestartTeleportKubeAgentPods(context.Background(), log, fakeClient)
			if err != nil {
				t.Fatalf("RestartTeleportKubeAgentPods() error = %v", err)
			}

			// Verify pods were deleted
			podList := &corev1.PodList{}
			err = fakeClient.List(context.Background(), podList, client.InNamespace(key.TeleportKubeAppNamespace))
			if err != nil {
				t.Fatalf("Failed to list pods: %v", err)
			}

			remainingAgentPods := 0
			for _, pod := range podList.Items {
				if teleportClient.isTeleportKubeAgentPod(pod.Name) {
					remainingAgentPods++
				}
			}

			if remainingAgentPods != 0 {
				t.Errorf("Expected 0 remaining agent pods, got %d", remainingAgentPods)
			}

			// Count total remaining pods (should be original count minus expected deleted)
			expectedRemaining := len(tt.existingPods) - tt.expectedDeleted
			if len(podList.Items) != expectedRemaining {
				t.Errorf("Expected %d remaining pods, got %d", expectedRemaining, len(podList.Items))
			}
		})
	}
}

func TestIsTeleportKubeAgentStateSecret(t *testing.T) {
	tests := []struct {
		name       string
		secretName string
		expected   bool
	}{
		{
			name:       "Valid state secret",
			secretName: "teleport-kube-agent-0-state",
			expected:   true,
		},
		{
			name:       "Another valid state secret",
			secretName: "teleport-kube-agent-10-state",
			expected:   true,
		},
		{
			name:       "Invalid - no state suffix",
			secretName: "teleport-kube-agent-0",
			expected:   false,
		},
		{
			name:       "Invalid - wrong prefix",
			secretName: "other-kube-agent-0-state",
			expected:   false,
		},
		{
			name:       "Invalid - empty string",
			secretName: "",
			expected:   false,
		},
		{
			name:       "Invalid - different suffix",
			secretName: "teleport-kube-agent-0-config",
			expected:   false,
		},
	}

	cfg := &config.Config{}
	teleportClient := New("test-namespace", cfg, token.NewGenerator())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := teleportClient.isTeleportKubeAgentStateSecret(tt.secretName)
			if result != tt.expected {
				t.Errorf("isTeleportKubeAgentStateSecret(%s) = %v, expected %v", tt.secretName, result, tt.expected)
			}
		})
	}
}

func TestIsTeleportKubeAgentPod(t *testing.T) {
	tests := []struct {
		name     string
		podName  string
		expected bool
	}{
		{
			name:     "Valid agent pod",
			podName:  "teleport-kube-agent-0",
			expected: true,
		},
		{
			name:     "Another valid agent pod",
			podName:  "teleport-kube-agent-10",
			expected: true,
		},
		{
			name:     "Invalid - state suffix (not a pod)",
			podName:  "teleport-kube-agent-0-state",
			expected: false,
		},
		{
			name:     "Invalid - wrong prefix",
			podName:  "other-kube-agent-0",
			expected: false,
		},
		{
			name:     "Invalid - empty string",
			podName:  "",
			expected: false,
		},
		{
			name:     "Invalid - different suffix",
			podName:  "teleport-kube-agent-0-config",
			expected: false,
		},
	}

	cfg := &config.Config{}
	teleportClient := New("test-namespace", cfg, token.NewGenerator())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := teleportClient.isTeleportKubeAgentPod(tt.podName)
			if result != tt.expected {
				t.Errorf("isTeleportKubeAgentPod(%s) = %v, expected %v", tt.podName, result, tt.expected)
			}
		})
	}
}
