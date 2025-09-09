/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"
)

func TestConfigReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = capi.AddToScheme(scheme)

	testCases := []struct {
		name            string
		configMapName   string
		configMapData   map[string]string
		configMapExists bool
		expectError     bool
		expectRequeue   bool
	}{
		{
			name:          "ConfigMap updated - triggers reconciliation",
			configMapName: key.TeleportOperatorConfigName,
			configMapData: map[string]string{
				"proxyAddr":             "proxy.example.com:443",
				"teleportVersion":       "17.0.0",
				"managementClusterName": "management",
				"appName":               "teleport-kube-agent",
				"appVersion":            "0.12.0",
				"appCatalog":            "giantswarm",
			},
			configMapExists: true,
			expectError:     false,
			expectRequeue:   false,
		},
		{
			name:            "ConfigMap deleted - no action needed",
			configMapName:   key.TeleportOperatorConfigName,
			configMapExists: false,
			expectError:     false,
			expectRequeue:   false,
		},
		{
			name:          "Different ConfigMap - still triggers reconciliation",
			configMapName: "other-configmap",
			configMapData: map[string]string{
				"somedata": "value",
			},
			configMapExists: true,
			expectError:     false,
			expectRequeue:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test objects
			objects := []runtime.Object{}

			// Add a test cluster to verify reconciliation triggering
			testCluster := &capi.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
			}
			objects = append(objects, testCluster)

			// Add ConfigMap if it exists
			if tc.configMapExists {
				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tc.configMapName,
						Namespace: "test-namespace",
					},
					Data: tc.configMapData,
				}
				objects = append(objects, configMap)
			}

			// Always add the teleport-operator ConfigMap for config parsing
			// unless we're testing its deletion
			if tc.configMapName != key.TeleportOperatorConfigName || tc.configMapExists {
				teleportConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      key.TeleportOperatorConfigName,
						Namespace: "test-namespace",
					},
					Data: map[string]string{
						"proxyAddr":             "proxy.example.com:443",
						"teleportVersion":       "17.0.0",
						"managementClusterName": "management",
						"appName":               "teleport-kube-agent",
						"appVersion":            "0.12.0",
						"appCatalog":            "giantswarm",
					},
				}
				// Only add if we haven't already added it above
				if tc.configMapName != key.TeleportOperatorConfigName {
					objects = append(objects, teleportConfigMap)
				}
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			reconciler := &ConfigReconciler{
				Client:    client,
				Log:       logr.Discard(),
				Scheme:    scheme,
				Teleport:  &teleport.Teleport{},
				Namespace: "test-namespace",
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tc.configMapName,
					Namespace: "test-namespace",
				},
			}

			// Execute reconcile
			result, err := reconciler.Reconcile(context.Background(), req)

			// Verify results
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tc.expectRequeue && result.Requeue == false {
				t.Error("Expected requeue but got none")
			}
			if !tc.expectRequeue && result.Requeue == true {
				t.Error("Expected no requeue but got requeue")
			}

			// For successful ConfigMap updates, verify cluster annotation was added
			// Note: Any ConfigMap change triggers reconciliation, not just teleport-operator ConfigMap
			// because the predicate filtering happens at the manager level, not in Reconcile
			if tc.expectError == false && tc.configMapExists {
				updatedCluster := &capi.Cluster{}
				err := client.Get(context.Background(), types.NamespacedName{
					Name:      "test-cluster",
					Namespace: "default",
				}, updatedCluster)
				
				if err != nil {
					t.Errorf("Failed to get updated cluster: %v", err)
				} else {
					if updatedCluster.Annotations == nil || updatedCluster.Annotations[key.ConfigUpdateAnnotation] == "" {
						t.Error("Expected config update annotation on cluster but found none")
					}
				}
			}
		})
	}
}
