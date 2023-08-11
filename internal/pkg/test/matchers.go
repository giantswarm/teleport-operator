package test

import (
	"fmt"
	"reflect"
	"testing"

	appv1alpha1 "github.com/giantswarm/apiextensions-application/api/v1alpha1"
	"github.com/gravitational/teleport/api/types"
	corev1 "k8s.io/api/core/v1"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

func CheckCluster(t *testing.T, expected, actual *capi.Cluster) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		t.Fatalf("clusters do not match:\nexpected nil\nactual %v", actual)
	}
	if actual == nil {
		t.Fatalf("clusters do not match:\nexpected %v\nactual nil", expected)
	}

	expectedName := fmt.Sprintf("%s/%s", expected.Namespace, expected.Name)
	actualName := fmt.Sprintf("%s/%s", actual.Namespace, actual.Name)

	if expectedName != actualName || !reflect.DeepEqual(expected.Finalizers, actual.Finalizers) {
		t.Fatalf("clusters do not match:\nexpected %v\nactual %v", expected, actual)
	}
}

func CheckSecret(t *testing.T, expected, actual *corev1.Secret) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		t.Fatalf("secrets do not match:\nexpected nil\nactual %v", actual)
	}
	if actual == nil {
		t.Fatalf("secrets do not match:\nexpected %v\nactual nil", expected)
	}

	expectedName := fmt.Sprintf("%s/%s", expected.Namespace, expected.Name)
	actualName := fmt.Sprintf("%s/%s", actual.Namespace, actual.Name)

	expectedToken := expected.StringData[JoinTokenKey]
	actualToken := actual.StringData[JoinTokenKey]

	if expectedName != actualName || expectedToken != actualToken {
		t.Fatalf("secrets do not match:\nexpected %v\nactual %v", expected, actual)
	}
}

func CheckConfigMap(t *testing.T, expected, actual *corev1.ConfigMap) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		t.Fatalf("config maps do not match:\nexpected nil\nactual %v", actual)
	}
	if actual == nil {
		t.Fatalf("config maps do not match:\nexpected %v\nactual nil", expected)
	}

	expectedName := fmt.Sprintf("%s/%s", expected.Namespace, expected.Name)
	actualName := fmt.Sprintf("%s/%s", actual.Namespace, actual.Name)

	expectedValues := expected.Data["values"]
	actualValues := actual.Data["values"]

	if expectedName != actualName || expectedValues != actualValues {
		t.Fatalf("config maps do not match:\nexpected %v\nactual %v", expected, actual)
	}
}

func CheckApp(t *testing.T, expected, actual *appv1alpha1.App) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		t.Fatalf("apps do not match:\nexpected nil\nactual %v", actual)
	}
	if actual == nil {
		t.Fatalf("apps do not match:\nexpected %v\nactual nil", expected)
	}

	expectedName := fmt.Sprintf("%s/%s", expected.Namespace, expected.Name)
	actualName := fmt.Sprintf("%s/%s", actual.Namespace, actual.Name)

	expectedCatalog := expected.Spec.Catalog
	actualCatalog := actual.Spec.Catalog

	expectedAppName := fmt.Sprintf("%s/%s", expected.Spec.Namespace, expected.Spec.Name)
	actualAppName := fmt.Sprintf("%s/%s", actual.Spec.Namespace, actual.Spec.Name)

	expectedConfigMap := fmt.Sprintf("%s/%s", expected.Spec.UserConfig.ConfigMap.Namespace, expected.Spec.UserConfig.ConfigMap.Name)
	actualConfigMap := fmt.Sprintf("%s/%s", actual.Spec.UserConfig.ConfigMap.Namespace, actual.Spec.UserConfig.ConfigMap.Name)

	if expectedName != actualName || expectedCatalog != actualCatalog || expectedAppName != actualAppName || expectedConfigMap != actualConfigMap {
		t.Fatalf("apps do not match:\nexpected %v\nactual %v", expected, actual)
	}

	if !reflect.DeepEqual(expected.Labels, actual.Labels) {
		t.Fatalf("apps do not match:\nexpected %v\nactual %v", expected, actual)
	}

	if !reflect.DeepEqual(expected.Spec.KubeConfig, actual.Spec.KubeConfig) {
		t.Fatalf("apps do not match:\nexpected %v\nactual %v", expected, actual)
	}
}

func CheckTokens(t *testing.T, expected, actual []types.ProvisionToken) {
	if len(expected) != len(actual) {
		t.Fatalf("unexpected number of tokens: expected %d, actual %d", len(expected), len(actual))
	}

	for _, actualToken := range actual {
		isExpected := false
		for _, expectedToken := range expected {
			if expectedToken.GetName() == actualToken.GetName() {
				isExpected = true
				break
			}
		}
		if !isExpected {
			t.Fatalf("unexpected list of tokens: expected %v, actual %v", expected, actual)
		}
	}
}

func CheckToken(t *testing.T, expected, actual types.ProvisionToken) {
	if expected == nil && actual == nil {
		return
	}
	if expected == nil {
		t.Fatalf("tokens do not match:\nexpected nil\nactual %v", actual)
	}
	if actual == nil {
		t.Fatalf("tokens do not match:\nexpected %v\nactual nil", expected)
	}

	if expected.GetName() != actual.GetName() {
		t.Fatalf("tokens do not equal: expected %v, actual %v", expected, actual)
	}

	if expected.GetRoles().String() != actual.GetRoles().String() {
		t.Fatalf("tokens do not equal: expected %v, actual %v", expected, actual)
	}

	expectedLabels := expected.GetMetadata().Labels
	actualLabels := actual.GetMetadata().Labels
	if len(expectedLabels) != len(actualLabels) {
		t.Fatalf("tokens do not equal: expected %v, actual %v", expected, actual)
	}

	for key, expectedValue := range expectedLabels {
		actualValue := actualLabels[key]
		if expectedValue != actualValue {
			t.Fatalf("tokens do not equal: expected %v, actual %v", expected, actual)
		}
	}

}

func CheckK8sServers(t *testing.T, expected, actual []types.KubeServer) {
	if len(expected) != len(actual) {
		t.Fatalf("unexpected number of tokens: expected %d, actual %d", len(expected), len(actual))
	}

	for _, actualServer := range actual {
		isExpected := false
		for _, expectedServer := range expected {
			if expectedServer.GetName() == actualServer.GetName() {
				isExpected = true
				break
			}
		}
		if !isExpected {
			t.Fatalf("unexpected list of Kubernetes servers: expected %v, actual %v", expected, actual)
		}
	}
}

func CheckError(t *testing.T, expectError bool, err error) {
	if err != nil && !expectError {
		t.Fatalf("unexpected error %v", err)
	}
	if err == nil && expectError {
		t.Fatal("did not receive an expected error")
	}
}
