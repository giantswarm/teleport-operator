package key

import (
	"fmt"
	"time"
)

const (
	AppOperatorVersion              = "0.0.0"
	TeleportOperatorFinalizer       = "teleport.finalizer.giantswarm.io"
	TeleportKubeAppDefaultNamespace = "giantswarm"
	TeleportKubeAppNamespace        = "kube-system"
	TeleportOperatorLabelValue      = "teleport-operator"
	TeleportOperatorConfigName      = "teleport-operator"
	TeleportBotSecretName           = "identity-output"
	TeleportKubeTokenValidity       = 720 * time.Hour
	TeleportNodeTokenValidity       = 720 * time.Hour

	AppCatalog            = "appCatalog"
	AppName               = "appName"
	AppVersion            = "appVersion"
	IdentityFile          = "identityFile"
	Identity              = "identity"
	ManagementClusterName = "managementClusterName"
	ProxyAddr             = "proxyAddr"
	TeleportVersion       = "teleportVersion"
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

func GetConfigmapDataFromTemplate(authToken string, proxyAddr string, kubeClusterName string, teleportVersion string) string {
	dataTpl := `roles: "kube"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`

	if teleportVersion != "" {
		dataTpl = fmt.Sprintf("%steleportVersionOverride: %q", dataTpl, teleportVersion)
	}

	return fmt.Sprintf(dataTpl, authToken, proxyAddr, kubeClusterName)
}
