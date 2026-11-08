package main

// MU300 reports the currently observed SIM number, never the number saved by
// an older pairing. A known empty result must clear Relay's previous number.
// This only reads the poller's snapshot: no extra AT requests during a call.
func (a *agent) mu300HeartbeatPhoneNumber() *string {
	if a.profile.Name != "mu300-openwrt" {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	number := ""
	if a.modem.SIMInserted && a.modem.PhoneNumberStatus == "available" {
		number = a.modem.PhoneNumber
	}
	return &number
}
