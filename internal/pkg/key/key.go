package key

import (
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
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

	// TeleportKubeAgentValuesKey is the top-level key under which
	// teleport-kube-agent chart versions newer than v0.10.8 expect their
	// values to be nested.
	TeleportKubeAgentValuesKey = "teleport-kube-agent"

	// teleportKubeAgentNestedSinceVersion is the threshold above which the
	// chart switched to nested values.
	teleportKubeAgentNestedSinceVersion = "0.10.8"
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

// UsesNestedKubeAgentValues reports whether the teleport-kube-agent chart
// version expects its values nested under the `teleport-kube-agent` key.
// Versions strictly greater than v0.10.8 use the nested layout. An
// unparseable version is treated as the legacy (flat) layout.
func UsesNestedKubeAgentValues(appVersion string) bool {
	v, err := semver.NewVersion(strings.TrimPrefix(appVersion, "v"))
	if err != nil {
		return false
	}
	threshold := semver.MustParse(teleportKubeAgentNestedSinceVersion)
	return v.GreaterThan(threshold)
}

func GetConfigmapDataFromTemplate(authToken string, proxyAddr string, kubeClusterName string, teleportVersion string, roles []string, appVersion string) string {
	dataTpl := `roles: "%s"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`

	if teleportVersion != "" {
		dataTpl = fmt.Sprintf("%steleportVersionOverride: %q", dataTpl, teleportVersion)
	}

	body := fmt.Sprintf(dataTpl, RolesToString(roles), authToken, proxyAddr, kubeClusterName)
	if !UsesNestedKubeAgentValues(appVersion) {
		return body
	}

	var nested strings.Builder
	nested.WriteString(TeleportKubeAgentValuesKey)
	nested.WriteString(":\n")
	for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
		nested.WriteString("  ")
		nested.WriteString(line)
		nested.WriteString("\n")
	}
	return nested.String()
}

func GetTbotConfigmapDataFromTemplate(kubeClusterName string, clusterName string) string {
	dataTpl := `outputs:
  %s: "%s"
`

	return fmt.Sprintf(dataTpl, kubeClusterName, clusterName)
}
