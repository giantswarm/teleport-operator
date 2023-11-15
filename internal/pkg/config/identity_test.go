package config

import (
	"context"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"
)

func Test_GetIdentityConfigFromSecret(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		secret         *corev1.Secret
		expectedConfig *IdentityConfig
		expectError    bool
	}{
		{
			name:      "case 0: Return identity config in case a valid secret exists",
			namespace: test.NamespaceName,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "identity-output",
					Namespace: test.NamespaceName,
				},
				Data: map[string][]byte{
					"identity": []byte(test.IdentityFileValue),
				},
			},
			expectedConfig: &IdentityConfig{
				IdentityFile: test.IdentityFileValue,
			},
		},
		{
			name:        "case 1: Fail in case the identity config secret does not exist",
			namespace:   test.NamespaceName,
			expectError: true,
		},
		{
			name:      "case 2: Fail in case the identity config secret exists but does not contain all keys",
			namespace: test.NamespaceName,
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.TeleportBotSecretName,
					Namespace: test.NamespaceName,
				},
				Data: map[string][]byte{},
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.secret != nil {
				runtimeObjects = append(runtimeObjects, tc.secret)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			actualConfig, err := GetIdentityConfigFromSecret(ctx, ctrlClient, tc.namespace)
			test.CheckError(t, tc.expectError, err)

			if err == nil && tc.expectedConfig.IdentityFile != actualConfig.IdentityFile {
				t.Fatalf("configs do not match: expected\n%v,\nactual\n%v", tc.expectedConfig, actualConfig)
			}
		})
	}
}
