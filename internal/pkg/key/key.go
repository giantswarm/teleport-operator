package key

import (
	"fmt"
	"strings"
	"time"

	"github.com/gravitational/teleport/api/types"
)

const (
	AppOperatorVersion              = "0.0.0"
	TeleportOperatorFinalizer       = "teleport.finalizer.giantswarm.io"
	TeleportKubeAppDefaultNamespace = "giantswarm"
	TeleportKubeAppNamespace        = "kube-system"
	TeleportOperatorLabelValue      = "teleport-operator"
	TeleportOperatorConfigName      = "teleport-operator"
	TeleportBotSecretName           = "identity-output"
	TeleportBotNamespace            = "giantswarm"
	TeleportBotAppName              = "teleport-tbot"
	TeleportAppTokenValidity        = 720 * time.Hour
	TeleportKubeTokenValidity       = 720 * time.Hour
	TeleportNodeTokenValidity       = 720 * time.Hour
	ConfigUpdateAnnotation          = "teleport-operator.giantswarm.io/config-updated"

	AppCatalog            = "appCatalog"
	AppName               = "appName"
	AppVersion            = "appVersion"
	IdentityFile          = "identityFile"
	Identity              = "identity"
	ManagementClusterName = "managementClusterName"
	ProxyAddr             = "proxyAddr"
	TeleportVersion       = "teleportVersion"
	RoleKube              = "kube"
	RoleApp               = "app"
	RoleNode              = "node"
)

func ParseRoles(s string) ([]string, error) {
	parts := strings.Split(s, ",")
	roles := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case RoleKube, RoleApp, RoleNode:
			roles = append(roles, part)
		default:
			return nil, fmt.Errorf("invalid role: %s", part)
		}
	}
	return roles, nil
}

func RolesToString(roles []string) string {
	return strings.Join(roles, ",")
}

func RolesToSystemRoles(roles []string) []types.SystemRole {
	systemRoles := make([]types.SystemRole, 0, len(roles))
	for _, role := range roles {
		switch role {
		case RoleKube:
			systemRoles = append(systemRoles, types.RoleKube)
		case RoleApp:
			systemRoles = append(systemRoles, types.RoleApp)
		case RoleNode:
			systemRoles = append(systemRoles, types.RoleNode)
		}
	}
	return systemRoles
}

func GetConfigmapName(clusterName string, appName string) string {
	return fmt.Sprintf("%s-%s-config", clusterName, appName)
}

func GetTbotConfigmapName(clusterName string) string {
	return fmt.Sprintf("teleport-tbot-%s-config", clusterName)
}

func GetSecretName(clusterName string) string {
	return fmt.Sprintf("%s-teleport-join-token", clusterName)
}

func GetKubeconfigSecretName(clusterName string) string {
	return fmt.Sprintf("teleport-%s-kubeconfig", clusterName)
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

func GetConfigmapDataFromTemplate(authToken string, proxyAddr string, kubeClusterName string, teleportVersion string, roles []string) string {
	dataTpl := `roles: "%s"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`

	if teleportVersion != "" {
		dataTpl = fmt.Sprintf("%steleportVersionOverride: %q", dataTpl, teleportVersion)
	}

	return fmt.Sprintf(dataTpl, RolesToString(roles), authToken, proxyAddr, kubeClusterName)
}

func GetTbotConfigmapDataFromTemplate(kubeClusterName string, clusterName string) string {
	dataTpl := `outputs:
  %s: "%s"
`

	return fmt.Sprintf(dataTpl, kubeClusterName, clusterName)
}
