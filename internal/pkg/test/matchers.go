package test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/gravitational/teleport/api/types"
	"gopkg.in/yaml.v3"
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
	if expectedName != actualName {
		t.Fatalf("config maps do not match:\nexpected %v\nactual %v", expected, actual)
	}

	var expectedValues map[string]string
	err := yaml.Unmarshal([]byte(expected.Data["values"]), &expectedValues)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	var actualValues map[string]string
	err = yaml.Unmarshal([]byte(actual.Data["values"]), &actualValues)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	if !reflect.DeepEqual(expectedValues, actualValues) {
		t.Fatalf("config maps do not match:\nexpected %v\nactual %v", expected, actual)
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
