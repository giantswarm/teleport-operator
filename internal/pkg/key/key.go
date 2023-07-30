package key

import (
	"fmt"
)

func ConfigmapName(appName string) string {
	return fmt.Sprintf("%s-config", appName)
}

func SecretName(clusterName string) string {
	return fmt.Sprintf("%s-teleport-join-token", clusterName)
}

func RegisterName(managementClusterName, clusterName string) string {
	return fmt.Sprintf("%s-%s", managementClusterName, clusterName)
}

func AppSpecKubeConfigSecretName(clusterName string) string {
	return fmt.Sprintf("%s-kubeconfig", clusterName)
}
