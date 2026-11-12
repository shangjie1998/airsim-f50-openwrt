package main

import "testing"

func txtMap(txt []string) map[string]string {
	m := make(map[string]string, len(txt))
	for _, kv := range txt {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				m[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return m
}

func newAdDaemon() *daemon {
	return &daemon{deviceID: "mu300-test", settings: managementSettings{ControlPort: 8581, PCMPort: 8582}}
}

// 没有隧道地址：TXT 里一个 alt 字段都不该有，手机侧行为跟以前完全一致。
func TestAdvertisementWithoutTunnelHasNoAltFields(t *testing.T) {
	d := newAdDaemon()
	m := txtMap(d.advertisementText("192.168.50.1"))
	for _, k := range []string{"alt_host", "alt_control_port", "alt_pcm_port"} {
		if _, ok := m[k]; ok {
			t.Fatalf("%s should be absent without a tunnel: %v", k, m)
		}
	}
	if m["host"] != "192.168.50.1" || m["v"] != "1" {
		t.Fatalf("base fields broken: %v", m)
	}
}

// 有隧道地址：三个字段必须同时出现，且 alt_host 与主 host 不同。
func TestAdvertisementWithTunnelAddsAltTriplet(t *testing.T) {
	d := newAdDaemon()
	d.tunnel = "100.64.1.2"
	m := txtMap(d.advertisementText("192.168.50.1"))
	if m["alt_host"] != "100.64.1.2" {
		t.Fatalf("alt_host = %q", m["alt_host"])
	}
	if m["alt_control_port"] != "8581" || m["alt_pcm_port"] != "8582" {
		t.Fatalf("alt ports = %q/%q", m["alt_control_port"], m["alt_pcm_port"])
	}
}

// 隧道地址和主 host 相同时不写 alt（iOS 里 alt_host != host 才算数，写了也白写）。
func TestAdvertisementAltSkippedWhenSameAsHost(t *testing.T) {
	d := newAdDaemon()
	d.tunnel = "192.168.50.1"
	if _, ok := txtMap(d.advertisementText("192.168.50.1"))["alt_host"]; ok {
		t.Fatalf("alt_host should be omitted when it equals host")
	}
}
