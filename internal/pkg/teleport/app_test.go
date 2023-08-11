package teleport

import (
	"context"
	"testing"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/test"
	"github.com/giantswarm/teleport-operator/internal/pkg/token"
)

func Test_InstallKubeAgentApp(t *testing.T) {
	testCases := []struct {
		name                string
		namespace           string
		appName             string
		appCatalog          string
		appVersion          string
		clusterName         string
		registerName        string
		isManagementCluster bool
		app                 *appv1alpha1.App
		expectedApp         *appv1alpha1.App
		expectError         bool
	}{
		{
			name:                "case 0: Create management cluster app",
			namespace:           test.NamespaceName,
			appName:             test.AppName,
			appCatalog:          test.AppCatalog,
			appVersion:          test.AppVersion,
			clusterName:         test.ManagementClusterName,
			registerName:        test.ManagementClusterName,
			isManagementCluster: true,
			expectedApp:         test.NewApp(test.ManagementClusterName, test.AppName, test.NamespaceName, true),
		},
		{
			name:                "case 1: Create workload cluster app",
			namespace:           test.NamespaceName,
			appName:             test.AppName,
			appCatalog:          test.AppCatalog,
			appVersion:          test.AppVersion,
			clusterName:         test.ClusterName,
			registerName:        key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			isManagementCluster: false,
			expectedApp:         test.NewApp(test.ClusterName, test.AppName, test.NamespaceName, false),
		},
		{
			name:                "case 2: Fail in case the app already exists",
			namespace:           test.NamespaceName,
			appName:             test.AppName,
			appCatalog:          test.AppCatalog,
			appVersion:          test.AppVersion,
			clusterName:         test.ClusterName,
			registerName:        key.GetRegisterName(test.ManagementClusterName, test.ClusterName),
			isManagementCluster: false,
			app:                 test.NewApp(test.ClusterName, test.AppName, test.NamespaceName, false),
			expectError:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.app != nil {
				runtimeObjects = append(runtimeObjects, tc.app)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()
			log := ctrl.Log.WithName("test")

			secretConfig := &SecretConfig{
				AppName:    tc.appName,
				AppCatalog: tc.appCatalog,
				AppVersion: tc.appVersion,
			}

			teleport := New(tc.namespace, secretConfig, token.NewGenerator())
			err = teleport.InstallKubeAgentApp(ctx, log, ctrlClient, tc.clusterName, tc.registerName, tc.namespace, tc.isManagementCluster)
			test.CheckError(t, tc.expectError, err)

			if err == nil {
				actual := &appv1alpha1.App{}
				err = ctrlClient.Get(ctx, test.ObjectKeyFromObjectMeta(tc.expectedApp.ObjectMeta), actual)
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}

				test.CheckApp(t, tc.expectedApp, actual)
			}
		})
	}
}

func Test_IsKubeAgentAppInstalled(t *testing.T) {
	testCases := []struct {
		name           string
		namespace      string
		appName        string
		appCatalog     string
		appVersion     string
		clusterName    string
		app            *appv1alpha1.App
		expectedResult bool
	}{
		{
			name:           "case 0: Return false in case the app does not exist",
			namespace:      test.NamespaceName,
			appName:        test.AppName,
			appCatalog:     test.AppCatalog,
			appVersion:     test.AppVersion,
			clusterName:    test.ManagementClusterName,
			expectedResult: false,
		},
		{
			name:           "case 1: Return true in case the app exists",
			namespace:      test.NamespaceName,
			appName:        test.AppName,
			appCatalog:     test.AppCatalog,
			appVersion:     test.AppVersion,
			clusterName:    test.ManagementClusterName,
			app:            test.NewApp(test.ManagementClusterName, test.AppName, test.NamespaceName, true),
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var runtimeObjects []runtime.Object
			if tc.app != nil {
				runtimeObjects = append(runtimeObjects, tc.app)
			}

			ctrlClient, err := test.NewFakeK8sClient(runtimeObjects)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			ctx := context.TODO()

			secretConfig := &SecretConfig{
				AppName:    tc.appName,
				AppCatalog: tc.appCatalog,
				AppVersion: tc.appVersion,
			}

			teleport := New(tc.namespace, secretConfig, token.NewGenerator())
			result, err := teleport.IsKubeAgentAppInstalled(ctx, ctrlClient, tc.clusterName, tc.namespace)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}

			if tc.expectedResult != result {
				t.Fatalf("unexpected result, expected %v, actual %v", tc.expectedResult, result)
			}
		})
	}
}
