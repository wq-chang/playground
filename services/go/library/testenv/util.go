package testenv

import (
	"fmt"
	"strings"
)

// SanitizeContainerName transforms a service and image name into a Docker-safe
// string suitable for use with testcontainers.WithReuseByName.
//
// It replaces common separators (:, /, .) with underscores to ensure the
// resulting name is a valid container identifier and prevents naming
// collisions or invalid character errors during local test execution.
func SanitizeContainerName(serviceName, imageName string) string {
	safeImageName := strings.NewReplacer(":", "_", "/", "_", ".", "_").Replace(imageName)

	return fmt.Sprintf("test_env_%s_%s", serviceName, safeImageName)
}
