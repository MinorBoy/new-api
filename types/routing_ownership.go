package types

import (
	"fmt"
	"strings"
)

type RouteTargetManagedBy string

const (
	RouteTargetManagedByManual       RouteTargetManagedBy = "manual"
	RouteTargetManagedByConfigImport RouteTargetManagedBy = "config_import"
)

func NormalizeRouteTargetManagedBy(value string) (RouteTargetManagedBy, error) {
	normalized := RouteTargetManagedBy(strings.ToLower(strings.TrimSpace(value)))
	if normalized == "" {
		return RouteTargetManagedByManual, nil
	}
	switch normalized {
	case RouteTargetManagedByManual, RouteTargetManagedByConfigImport:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid route target owner %q", value)
	}
}
