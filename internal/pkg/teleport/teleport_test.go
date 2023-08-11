package teleport

import (
	"context"
	"testing"

	teleportTypes "github.com/gravitational/teleport/api/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func TestTeleport_IsClusterRegisteredInTeleport(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		registerName   string
		servers        []teleportTypes.KubeServer
		failsList      bool
		expectedResult bool
		expectError    bool
	}{
		{
			name:           "case 0: Return true in case the cluster is known by Teleport",
			namespace:      test.NamespaceName,
			registerName:   key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			servers:        []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			expectedResult: true,
		},
		{
			name:           "case 1: Return false in case the cluster is not known by Teleport",
			namespace:      test.NamespaceName,
			registerName:   test.ManagementClusterName,
			servers:        []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			expectedResult: false,
		},
		{
			name:         "case 2: Fail in case Teleport client is unable to return a list of known clusters",
			namespace:    test.NamespaceName,
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			servers:      []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			failsList:    true,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teleport := New(tc.name, &SecretConfig{}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				KubernetesServers: tc.servers,
				FailsList:         tc.failsList,
			})

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			actualResult, err := teleport.IsClusterRegisteredInTeleport(ctx, log, tc.registerName)
			test.CheckError(t, tc.expectError, err)
			if err == nil && tc.expectedResult != actualResult {
				t.Fatalf("unexpected result: expected %v, actual %v", tc.expectedResult, actualResult)
			}
		})
	}
}

func Test_DeleteClusterFromTeleport(t *testing.T) {
	testCases := []struct {
		name         string
		registerName string
		servers      []teleportTypes.KubeServer
		failsList    bool
		failsDelete  bool
		expectError  bool
	}{
		{
			name:         "case 0: Succeed in case an existing cluster is deleted",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			servers:      []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			expectError:  false,
		},
		{
			name:         "case 1: Succeed when deleting a non-existent cluster",
			registerName: test.ManagementClusterName,
			servers:      []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			expectError:  false,
		},
		{
			name:         "case 2: Fail in case the Teleport client is unable to return a list of known clusters",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			servers:      []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			failsList:    true,
			expectError:  true,
		},
		{
			name:         "case 3: Fail in case the Teleport client is unable to delete the cluster",
			registerName: key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			servers:      []teleportTypes.KubeServer{test.NewKubeServer(test.ClusterName, "host-id", "host-name")},
			failsDelete:  true,
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			teleport := New(tc.name, &SecretConfig{}, token.NewGenerator())
			teleport.TeleportClient = test.NewTeleportClient(test.FakeTeleportClientConfig{
				KubernetesServers: tc.servers,
				FailsList:         tc.failsList,
				FailsDelete:       tc.failsDelete,
			})

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			err := teleport.DeleteClusterFromTeleport(ctx, log, tc.registerName)
			test.CheckError(t, tc.expectError, err)
		})
	}
}
