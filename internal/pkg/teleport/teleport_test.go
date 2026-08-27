package teleport

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func newUserValuesConfigMap(clusterName, namespaceName, values string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-teleport-kube-agent-user-values", clusterName),
			Namespace: namespaceName,
		},
		Data: map[string]string{"values": values},
	}
}

const appsList = `
- name: grafana-test
  uri: http://grafana.monitoring.svc.cluster.local:80
  public_addr: grafana-test.teleport.giantswarm.io
`

func Test_AreTeleportAppsEnabled(t *testing.T) {
	testCases := []struct {
		name        string
		values      string
		noConfigMap bool
		noValuesKey bool
		expected    bool
		expectError bool
	}{
		{
			name:        "case 0: no ConfigMap at all",
			noConfigMap: true,
			expected:    false,
		},
		{
			name:        "case 1: ConfigMap without a values key",
			noValuesKey: true,
			expected:    false,
		},
		{
			name:     "case 2: flat apps only (pre-v35 layout)",
			values:   "apps:" + appsList,
			expected: true,
		},
		{
			name:     "case 3: nested apps only (chart v0.11.0+ layout)",
			values:   "teleport-kube-agent:\n  apps:\n  - name: grafana-test\n    uri: http://grafana:80\n",
			expected: true,
		},
		{
			name: "case 4: both layouts, as written during the migration",
			values: "apps:" + appsList +
				"teleport-kube-agent:\n  apps:\n  - name: grafana-test\n    uri: http://grafana:80\n",
			expected: true,
		},
		{
			name:     "case 5: a nested block for an unrelated setting must not hide the flat apps",
			values:   "apps:" + appsList + "teleport-kube-agent:\n  podMonitor:\n    enabled: false\n",
			expected: true,
		},
		{
			name:     "case 6: no apps in either layout",
			values:   "teleport-kube-agent:\n  podMonitor:\n    enabled: false\n",
			expected: false,
		},
		{
			name:     "case 7: empty flat apps list",
			values:   "apps: []\n",
			expected: false,
		},
		{
			name:     "case 8: empty nested apps list",
			values:   "teleport-kube-agent:\n  apps: []\n",
			expected: false,
		},
		{
			name:        "case 9: malformed YAML",
			values:      "apps: [oh no\n",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if !tc.noConfigMap {
				cm := newUserValuesConfigMap(test.ClusterName, test.NamespaceName, tc.values)
				if tc.noValuesKey {
					cm.Data = map[string]string{}
				}
				runtimeObjects = append(runtimeObjects, cm)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			teleport := New(test.NamespaceName, &config.Config{AppName: test.AppName}, token.NewGenerator())
			teleport.Client = ctrlClient

			actual, err := teleport.AreTeleportAppsEnabled(context.TODO(), test.ClusterName, test.NamespaceName)
			test.CheckError(t, tc.expectError, err)
			if err != nil {
				return
			}
			if actual != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
