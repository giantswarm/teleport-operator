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
	// teleport-kube-agent v0.11.0+ reads its values.
	TeleportKubeAgentValuesKey = "teleport-kube-agent"

	// teleportKubeAgentNestedSinceVersion is the lowest chart version that
	// reads its values nested under TeleportKubeAgentValuesKey.
	teleportKubeAgentNestedSinceVersion = "0.11.0"

	// teleportKubeAgentBundledTeleportVersion is the Teleport version that
	// teleport-kube-agent v0.11.0 bundles by default. The nested block
	// skips teleportVersionOverride when the operator-configured override
	// would be a downgrade against this bundled version.
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

// ResolveNestedTeleportVersionOverride returns the value to write as
// `teleportVersionOverride` inside the nested `teleport-kube-agent:` block,
// or "" when the override should be omitted. The nested block always
// targets chart v0.11.0+ consumers (whether emitted alone or alongside a
// flat block for upgrade safety), so the floor applies unconditionally:
// any teleportVersion below teleportKubeAgentBundledTeleportVersion — or
// one that doesn't parse as semver — is dropped to avoid a downgrade
// against the chart's bundled Teleport.
func ResolveNestedTeleportVersionOverride(teleportVersion string) string {
	if teleportVersion == "" {
		return ""
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

// GetConfigmapDataFromTemplate renders the teleport-kube-agent values document
// for a cluster. The layout depends on the cluster's deployed chart version:
//
//   - tkaVersion >= v0.11.0: a single nested block under TeleportKubeAgentValuesKey,
//     because that's the only place the chart looks.
//   - tkaVersion < v0.11.0 or unknown ("" / unparseable): the legacy flat
//     layout at the root, PLUS the nested block. The flat block keeps the
//     chart working today; the nested block is there so an in-place upgrade
//     past v0.11.0 doesn't lose values in the window before the operator's
//     next reconcile.
//
// teleportVersionOverride is passed through in the flat block when non-empty,
// but the nested block applies a floor (see ResolveTeleportVersionOverride):
// the override is dropped if it would be a downgrade against the v0.11.0
// chart's bundled Teleport version.
func GetConfigmapDataFromTemplate(authToken, proxyAddr, kubeClusterName, teleportVersion string, roles []string, tkaVersion string) string {
	flat := renderFlatValuesBlock(authToken, proxyAddr, kubeClusterName, teleportVersion, roles)
	nestedOverride := ResolveNestedTeleportVersionOverride(teleportVersion)
	nested := renderNestedValuesBlock(authToken, proxyAddr, kubeClusterName, nestedOverride, roles)

	if UsesNestedKubeAgentValues(tkaVersion) {
		return nested
	}
	return flat + nested
}

func renderFlatValuesBlock(authToken, proxyAddr, kubeClusterName, teleportVersion string, roles []string) string {
	body := fmt.Sprintf(`roles: "%s"
authToken: "%s"
proxyAddr: "%s"
kubeClusterName: "%s"
`, RolesToString(roles), authToken, proxyAddr, kubeClusterName)
	if teleportVersion != "" {
		body += fmt.Sprintf("teleportVersionOverride: %q\n", teleportVersion)
	}
	return body
}

func renderNestedValuesBlock(authToken, proxyAddr, kubeClusterName, teleportVersion string, roles []string) string {
	body := fmt.Sprintf(`%s:
  roles: "%s"
  authToken: "%s"
  proxyAddr: "%s"
  kubeClusterName: "%s"
`, TeleportKubeAgentValuesKey, RolesToString(roles), authToken, proxyAddr, kubeClusterName)
	if teleportVersion != "" {
		body += fmt.Sprintf("  teleportVersionOverride: %q\n", teleportVersion)
	}
	return body
}

func GetTbotConfigmapDataFromTemplate(kubeClusterName string, clusterName string) string {
	dataTpl := `outputs:
  %s: "%s"
`

	return fmt.Sprintf(dataTpl, kubeClusterName, clusterName)
}
