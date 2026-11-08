package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMU300ManagementPolicy(t *testing.T) {
	t.Setenv("AIRSIM_RUNTIME_PROFILE", "mu300-openwrt")
	t.Setenv("AIRSIM_MU300_MODE", "local")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(200) }))
	defer server.Close()
	p := &pushManager{client: server.Client(), config: pushRegistration{RelayURL: server.URL, DeviceID: "fixture", DeviceSecret: "fixture-secret"}}
	if p.cloudEnabledLocked() {
		t.Fatal("local mode enabled cloud")
	}
	if e := p.sendRegistration(); e != nil || requests.Load() != 0 {
		t.Fatal("local registration contacted Relay")
	}
	if _, ready := p.commandRegistration(); ready {
		t.Fatal("local commands enabled")
	}
	t.Setenv("AIRSIM_MU300_MODE", "cloud")
	t.Setenv("AIRSIM_MU300_RELAY", "https://new-relay.example")
	if p.cloudEnabledLocked() || p.status().ManagementBlocked != "relay_pairing_required" {
		t.Fatal("old credentials allowed for new Relay")
	}
	if e := p.sendRegistration(); e != nil || requests.Load() != 0 {
		t.Fatal("mismatched Relay received credentials")
	}
	t.Setenv("AIRSIM_MU300_RELAY", server.URL)
	if !p.cloudEnabledLocked() {
		t.Fatal("matching Relay blocked")
	}
	if e := p.sendRegistration(); e != nil || requests.Load() != 1 {
		t.Fatal("matching registration failed")
	}
	p.config.LocalMode = true
	if p.cloudEnabledLocked() {
		t.Fatal("cloud mode escalated local pairing")
	}
	p.config.LocalMode = false
	t.Setenv("AIRSIM_RUNTIME_PROFILE", "android-avf")
	t.Setenv("AIRSIM_MU300_MODE", "local")
	if !p.cloudEnabledLocked() {
		t.Fatal("MU300 policy changed other profiles")
	}
}
