package key

import (
	"fmt"
	"time"
)

const (
	AppOperatorVersion            = "0.0.0"
	TeleportOperatorFinalizer     = "teleport.finalizer.giantswarm.io"
	MCTeleportAppDefaultNamespace = "giantswarm"
	TeleportKubeAppNamespace      = "kube-system"
	TeleportOperatorLabelValue    = "teleport-operator"
	TeleportOperatorSecretName    = "teleport-operator"
	TeleportTokenValidity         = 24 * time.Hour
)

func GetConfigmapName(clusterName string, appName string) string {
	return fmt.Sprintf("%s-%s-config", clusterName, appName)
}

func GetSecretName(clusterName string) string {
	return fmt.Sprintf("%s-teleport-join-token", clusterName)
}

func GetRegisterName(managementClusterName, clusterName string) string {
	return fmt.Sprintf("%s-%s", managementClusterName, clusterName)
}

func GetAppSpecKubeConfigSecretName(clusterName string) string {
	return fmt.Sprintf("%s-kubeconfig", clusterName)
}

func GetAppName(clusterName string, appName string) string {
	return fmt.Sprintf("%s-%s", clusterName, appName)
}
