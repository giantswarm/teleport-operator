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
	// teleport-kube-agent chart versions v0.11.0 and newer expect their
	// values to be nested.
	TeleportKubeAgentValuesKey = "teleport-kube-agent"

	// teleportKubeAgentNestedSinceVersion is the lowest chart version that
	// expects the nested values layout.
	teleportKubeAgentNestedSinceVersion = "0.11.0"

	// teleportKubeAgentBundledTeleportVersion is the Teleport version
	// bundled by teleport-kube-agent v0.11.0. With the nested-layout chart
	// we skip teleportVersionOverride when the configured override would
	// be a downgrade against this bundled version.
	teleportKubeAgentBundledTeleportVersion = "18.7.6"
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
// Versions at or above v0.11.0 use the nested layout. An unparseable
// version is treated as the legacy (flat) layout.
func UsesNestedKubeAgentValues(appVersion string) bool {
	v, err := semver.NewVersion(strings.TrimPrefix(appVersion, "v"))
	if err != nil {
		return false
	}
	threshold := semver.MustParse(teleportKubeAgentNestedSinceVersion)
	return v.Compare(threshold) >= 0
}

// ResolveTeleportVersionOverride returns the value that should be written
// as `teleportVersionOverride` for a given chart appVersion and operator
// teleportVersion, or an empty string when the override should be omitted.
//
// For the nested-layout chart (>= v0.11.0) the chart already bundles
// Teleport v18.7.6, so an override below that would be a downgrade and is
// skipped. An unparseable teleportVersion is also skipped in that case.
// Older flat-layout charts keep the legacy behaviour: any non-empty value
// is written through.
func ResolveTeleportVersionOverride(appVersion, teleportVersion string) string {
	if teleportVersion == "" {
		return ""
	}
	if !UsesNestedKubeAgentValues(appVersion) {
		return teleportVersion
	}

	v, err := semver.NewVersion(strings.TrimPrefix(teleportVersion, "v"))
	if err != nil {
		return ""
	}
	bundled := semver.MustParse(teleportKubeAgentBundledTeleportVersion)
	if v.Compare(bundled) < 0 {
		return ""
	}
	return teleportVersion
}

func GetConfigmapDataFromTemplate(authToken string, proxyAddr string, kubeClusterName string, teleportVersion string, roles []string, appVersion string) string {
	dataTpl := `roles: "%s"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`

	if override := ResolveTeleportVersionOverride(appVersion, teleportVersion); override != "" {
		dataTpl = fmt.Sprintf("%steleportVersionOverride: %q", dataTpl, override)
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
