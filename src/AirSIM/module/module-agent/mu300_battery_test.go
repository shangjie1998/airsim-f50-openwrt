package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func batteryFixture(t *testing.T, root, name, value string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(value), 0644); err != nil {
		t.Fatal(err)
	}
}

func u30BatteryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, value := range map[string]string{
		"etc/mu300-device":                        "u30air\n",
		"sys/class/power_supply/battery/type":     "Battery\n",
		"sys/class/power_supply/battery/present":  "1\n",
		"sys/class/power_supply/battery/capacity": "82\n",
		"sys/class/power_supply/battery/status":   "Charging\n",
	} {
		batteryFixture(t, root, name, value)
	}
	return root
}

func TestMU300BatteryNativeStatesAndZero(t *testing.T) {
	now := time.UnixMilli(1800000000000)
	for _, tc := range []struct {
		status, capacity string
		percent          int
		charging         bool
	}{
		{"Charging", "0", 0, true},
		{"Full", "100", 100, true},
		{"Discharging", "82", 82, false},
		{"Not charging", "82", 82, false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			root := u30BatteryFixture(t)
			batteryFixture(t, root, "sys/class/power_supply/battery/status", tc.status)
			batteryFixture(t, root, "sys/class/power_supply/battery/capacity", tc.capacity)
			v := readMU300Battery(root, now)
			if v == nil || v.Percent != tc.percent || v.Charging != tc.charging || v.ObservedAtMS != now.UnixMilli() {
				t.Fatalf("unexpected battery: %+v", v)
			}
		})
	}
}

func TestMU300BatteryRejectsUnknownAndSyntheticReadings(t *testing.T) {
	for _, tc := range []struct{ name, path, value string }{
		{"f50", "etc/mu300-device", "f50"},
		{"v50", "etc/mu300-device", "v50"},
		{"unknown board", "etc/mu300-device", ""},
		{"charger", "sys/class/power_supply/battery/type", "USB"},
		{"absent", "sys/class/power_supply/battery/present", "0"},
		{"invalid present", "sys/class/power_supply/battery/present", "unknown"},
		{"unknown status", "sys/class/power_supply/battery/status", "Unknown"},
		{"missing capacity", "sys/class/power_supply/battery/capacity", ""},
		{"fraction", "sys/class/power_supply/battery/capacity", "82.5"},
		{"negative", "sys/class/power_supply/battery/capacity", "-1"},
		{"raw gauge tenths", "sys/class/power_supply/battery/capacity", "820"},
		{"overfull", "sys/class/power_supply/battery/capacity", "101"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := u30BatteryFixture(t)
			batteryFixture(t, root, tc.path, tc.value)
			if v := readMU300Battery(root, time.Now()); v != nil {
				t.Fatalf("invented battery: %+v", v)
			}
		})
	}
	if v := readMU300Battery(t.TempDir(), time.Now()); v != nil {
		t.Fatal("missing hardware returned battery")
	}
}

func TestMU300BatteryDeviceTreeFallbackAndNoStaleCache(t *testing.T) {
	root := u30BatteryFixture(t)
	marker := filepath.Join(root, "etc/mu300-device")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if v := readMU300Battery(root, time.Now()); v != nil {
		t.Fatal("unidentified board accepted")
	}
	if err := os.MkdirAll(filepath.Join(root, "sys/firmware/devicetree/base/charger_policy_service"), 0755); err != nil {
		t.Fatal(err)
	}
	if v := readMU300Battery(root, time.Now()); v == nil {
		t.Fatal("known U30 tree rejected")
	}
	batteryFixture(t, root, "etc/mu300-device", "f50")
	if v := readMU300Battery(root, time.Now()); v != nil {
		t.Fatal("explicit other board ignored")
	}
	batteryFixture(t, root, "etc/mu300-device", "u30air")
	if err := os.Remove(filepath.Join(root, "sys/class/power_supply/battery/capacity")); err != nil {
		t.Fatal(err)
	}
	if v := readMU300Battery(root, time.Now()); v != nil {
		t.Fatal("previous sample retained after read failed")
	}
}

func TestMU300BatteryHealthIsProfileScopedAndOptional(t *testing.T) {
	root := u30BatteryFixture(t)
	for _, profile := range []string{"mu300-openwrt", "android-avf", "qdc507"} {
		a := &agent{mu300BatteryRoot: root, profile: runtimeProfile{Name: profile}, at: newATPort("unused"), started: time.Now()}
		w := httptest.NewRecorder()
		a.health(w, httptest.NewRequest("GET", "/api/health", nil))
		var body map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if (body["battery"] != nil) != (profile == "mu300-openwrt") {
			t.Fatal("battery leaked across profiles")
		}
	}
}

func TestMU300BatteryUsesExistingHeartbeatAndCloudPolicy(t *testing.T) {
	var received []map[string]json.RawMessage
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/heartbeat" {
			t.Error("new battery request channel")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		received = append(received, body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	m := newPushManager(filepath.Join(t.TempDir(), "push.json"))
	m.client = server.Client()
	if err := m.store(testPushRegistration(server.URL)); err != nil {
		t.Fatal(err)
	}
	a := &agent{mu300BatteryRoot: u30BatteryFixture(t), profile: runtimeProfile{Name: "mu300-openwrt"}}
	sample := a.mu300BatterySnapshot()
	if err := m.sendHeartbeat(agentHeartbeatSnapshot{CellularState: "registered", Battery: sample}); err != nil {
		t.Fatal(err)
	}
	if err := m.sendHeartbeat(agentHeartbeatSnapshot{CellularState: "registered"}); err != nil {
		t.Fatal(err)
	}
	if err := m.setCloudEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := m.sendHeartbeat(agentHeartbeatSnapshot{CellularState: "registered", Battery: sample}); err != nil {
		t.Fatal(err)
	}
	if len(received) != 2 {
		t.Fatal("cloud disabled still transmitted")
	}
	var value mu300BatteryReading
	if err := json.Unmarshal(received[0]["battery"], &value); err != nil {
		t.Fatal(err)
	}
	if value != *sample {
		t.Fatal("heartbeat changed battery sample")
	}
	if _, exists := received[1]["battery"]; exists {
		t.Fatal("unknown battery fabricated")
	}
}

func TestMU300BatteryUnknownUsesObservedChargerStatus(t *testing.T) {
	for _, tc := range []struct {
		status, online  string
		valid, charging bool
	}{
		{"Charging", "1", true, true}, {"Full", "1", true, true},
		{"Not charging", "1", true, false}, {"Discharging", "0", true, false},
		{"Charging", "0", false, false}, {"Unknown", "1", false, false},
		{"Charging", "", false, false}, {"Bad", "1", false, false},
	} {
		t.Run(tc.status+tc.online, func(t *testing.T) {
			root := u30BatteryFixture(t)
			batteryFixture(t, root, "sys/class/power_supply/battery/status", "Unknown")
			batteryFixture(t, root, "sys/class/power_supply/bq2560x_charger/status", tc.status)
			batteryFixture(t, root, "sys/class/power_supply/bq2560x_charger/online", tc.online)
			v := readMU300Battery(root, time.Now())
			if (v != nil) != tc.valid {
				t.Fatalf("valid=%v got=%v", tc.valid, v)
			}
			if v != nil && (v.Percent != 82 || v.Charging != tc.charging) {
				t.Fatal("wrong battery source", v)
			}
			batteryFixture(t, root, "etc/mu300-device", "f50")
			if readMU300Battery(root, time.Now()) != nil {
				t.Fatal("charger fallback escaped U30 profile")
			}
		})
	}
}

func TestMU300BatteryAuthoritativeBatteryStatusWins(t *testing.T) {
	root := u30BatteryFixture(t)
	batteryFixture(t, root, "sys/class/power_supply/battery/status", "Discharging")
	batteryFixture(t, root, "sys/class/power_supply/bq2560x_charger/status", "Charging")
	batteryFixture(t, root, "sys/class/power_supply/bq2560x_charger/online", "1")
	if v := readMU300Battery(root, time.Now()); v == nil || v.Charging {
		t.Fatal("charger overrode known battery state", v)
	}
}
