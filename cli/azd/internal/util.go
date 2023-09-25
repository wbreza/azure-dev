package internal

import (
	"strings"

	"github.com/azure/azure-dev/cli/azd/pkg/config"
)

func IsDevCenterEnabled(config config.Config) bool {
	devCenterModeNode, ok := config.Get("devCenter.mode")
	if !ok {
		return false
	}

	devCenterValue, ok := devCenterModeNode.(string)
	if !ok {
		return false
	}

	return strings.EqualFold(devCenterValue, "on")
}
