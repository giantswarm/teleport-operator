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
