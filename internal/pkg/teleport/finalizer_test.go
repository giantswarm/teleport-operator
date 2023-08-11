package teleport

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
)

func Test_AddFinalizer(t *testing.T) {
	testCases := []struct {
		name            string
		namespace       string
		cluster         *capi.Cluster
		expectedCluster *capi.Cluster
	}{
		{
			name:            "case 0: Add finalizer if it does not exist",
			namespace:       test.NamespaceName,
			cluster:         test.NewCluster(test.ClusterName, test.NamespaceName, nil, time.Time{}),
			expectedCluster: test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
		},
		{
			name:            "case 1: Do nothing in case the finalizer is already added",
			namespace:       test.NamespaceName,
			cluster:         test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
			expectedCluster: test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Time{}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.cluster != nil {
				runtimeObjects = append(runtimeObjects, tc.cluster)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			err = AddFinalizer(ctx, log, tc.cluster, ctrlClient)
			test.CheckError(t, false, err)

			actual := &capi.Cluster{}
			err = ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(tc.expectedCluster.ObjectMeta), actual)
			test.CheckError(t, false, err)

			test.CheckCluster(t, tc.expectedCluster, actual)
		})
	}
}

func Test_RemoveFinalizer(t *testing.T) {
	testCases := []struct {
		name            string
		namespace       string
		cluster         *capi.Cluster
		expectedCluster *capi.Cluster
	}{
		{
			name:            "case 0: Remove an existing teleport operator finalizer from a cluster",
			namespace:       test.NamespaceName,
			cluster:         test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer}, time.Now()),
			expectedCluster: nil,
		},
		{
			name:            "case 1: Remove teleport operator finalizer from a cluster, keep other finalizers",
			namespace:       test.NamespaceName,
			cluster:         test.NewCluster(test.ClusterName, test.NamespaceName, []string{key.TeleportOperatorFinalizer, "other-finalizer"}, time.Now()),
			expectedCluster: test.NewCluster(test.ClusterName, test.NamespaceName, []string{"other-finalizer"}, time.Now()),
		},
		{
			name:            "case 2: Do nothing in case the cluster does not have the teleport operator finalizers",
			namespace:       test.NamespaceName,
			cluster:         test.NewCluster(test.ClusterName, test.NamespaceName, []string{"other-finalizer"}, time.Now()),
			expectedCluster: test.NewCluster(test.ClusterName, test.NamespaceName, []string{"other-finalizer"}, time.Now()),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.cluster != nil {
				runtimeObjects = append(runtimeObjects, tc.cluster)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			err = RemoveFinalizer(ctx, log, tc.cluster, ctrlClient)
			test.CheckError(t, false, err)

			actual := &capi.Cluster{}
			err = ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(tc.cluster.ObjectMeta), actual)
			if err != nil && !errors.IsNotFound(err) {
				t.Fatalf("unexpected error %v", err)
			}

			if err != nil && tc.expectedCluster != nil {
				t.Fatalf("unexpected result: expected nil, actual %v", actual)
			}
			if err == nil && tc.expectedCluster == nil {
				t.Fatalf("unexpected result: expected %v, actual nil", tc.expectedCluster)
			}
			if err == nil {
				test.CheckCluster(t, tc.expectedCluster, actual)
			}
		})
	}
}
