package key

import (
	"fmt"
)

func ConfigmapName(appName string) string {
	return fmt.Sprintf("%s-config", appName)
}
