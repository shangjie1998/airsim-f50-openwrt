package main

import (
	"os"
	"strings"
)

// Device policy only restricts existing pairing permissions. It never replaces
// the destination of stored credentials and never enables a local-only pairing.
func mu300CloudPolicy(reg pushRegistration) string {
	if currentRuntimeProfile().Name != "mu300-openwrt" {
		return ""
	}
	switch os.Getenv("AIRSIM_MU300_MODE") {
	case "local":
		return "local_mode"
	case "", "cloud":
	default:
		return "invalid_mode"
	}
	selected := strings.TrimRight(os.Getenv("AIRSIM_MU300_RELAY"), "/")
	if selected != "" && selected != strings.TrimRight(reg.RelayURL, "/") {
		return "relay_pairing_required"
	}
	return ""
}
