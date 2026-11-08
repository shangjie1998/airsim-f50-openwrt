package main

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Matches AirSIM's existing battery metadata on health and relay heartbeats.
// This is the physical U30 Air battery, never a Linux guest's host battery.
type mu300BatteryReading struct {
	Percent      int   `json:"percent"`
	Charging     bool  `json:"charging"`
	ObservedAtMS int64 `json:"observed_at_ms"`
}

func (a *agent) mu300BatterySnapshot() *mu300BatteryReading {
	if a.profile.Name != "mu300-openwrt" {
		return nil
	}
	root := a.mu300BatteryRoot
	if root == "" {
		root = "/"
	}
	return readMU300Battery(root, time.Now())
}

// Read only the known U30 system-battery node. In particular, never pick the
// first power supply: F50/V50 may expose a synthetic gauge, and the raw SC27xx
// gauge on 5.4 uses tenths of a percent instead of the battery node's percent.
func readMU300Battery(root string, now time.Time) *mu300BatteryReading {
	device, err := readMU300BatteryText(filepath.Join(root, "etc/mu300-device"))
	if err == nil {
		if device != "u30air" {
			return nil
		}
	} else if os.IsNotExist(err) {
		// Native mu300-linux uses this U30-specific device-tree node when its
		// installer marker is absent. An explicit other-device marker wins.
		info, err := os.Stat(filepath.Join(root, "sys/firmware/devicetree/base/charger_policy_service"))
		if err != nil || !info.IsDir() {
			return nil
		}
	} else {
		return nil
	}
	base := filepath.Join(root, "sys/class/power_supply/battery")
	get := func(name string) (string, error) { return readMU300BatteryText(filepath.Join(base, name)) }
	if kind, err := get("type"); err != nil || kind != "Battery" {
		return nil
	}
	if present, err := get("present"); (err != nil && !os.IsNotExist(err)) || (err == nil && present != "1") {
		return nil
	}
	status, err := get("status")
	if err != nil {
		return nil
	}
	// U30's aggregate battery driver can expose a real capacity but Unknown
	// status. The observed BQ2560x charger is a separate authoritative source;
	// online alone does not imply charging (thermal/full/paused are possible).
	batteryStatus := status
	chargerBase := filepath.Join(root, "sys/class/power_supply/bq2560x_charger")
	chargerOnline := ""
	if status == "Unknown" {
		status, err = readMU300BatteryText(filepath.Join(chargerBase, "status"))
		if err != nil {
			return nil
		}
		chargerOnline, err = readMU300BatteryText(filepath.Join(chargerBase, "online"))
		if err != nil || (chargerOnline != "0" && chargerOnline != "1") {
			return nil
		}
	}
	charging := false
	switch status {
	case "Charging", "Full": // Same semantics as AirSIM's Android sender.
		charging = true
	case "Discharging", "Not charging":
	default:
		return nil
	}
	if batteryStatus == "Unknown" && charging && chargerOnline != "1" {
		return nil
	}
	raw, err := get("capacity")
	if err != nil {
		return nil
	}
	percent, err := strconv.Atoi(raw)
	if err != nil || percent < 0 || percent > 100 || now.UnixMilli() <= 0 {
		return nil
	}
	// A plug/unplug transition during the read is unknown until next refresh.
	if current, err := get("status"); err != nil || current != batteryStatus {
		return nil
	}
	if batteryStatus == "Unknown" {
		current, e := readMU300BatteryText(filepath.Join(chargerBase, "status"))
		online, oe := readMU300BatteryText(filepath.Join(chargerBase, "online"))
		if e != nil || oe != nil || current != status || online != chargerOnline {
			return nil
		}
	}
	return &mu300BatteryReading{Percent: percent, Charging: charging, ObservedAtMS: now.UnixMilli()}
}

func readMU300BatteryText(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 129))
	if err != nil {
		return "", err
	}
	if len(b) > 128 {
		return "", os.ErrInvalid
	}
	return strings.TrimSpace(string(b)), nil
}
