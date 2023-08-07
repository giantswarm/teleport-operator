package key

import (
	"crypto/rand"
	"encoding/hex"
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
	TeleportTokenLength           = 16
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

func CryptoRandomHex(length int) (string, error) {
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}
