package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"airsim.local/mu300/internal/audio"
	"airsim.local/mu300/internal/gateway"
	"airsim.local/mu300/internal/pairing"
	"github.com/grandcat/zeroconf"
)

var version = "0.1.0-dev"

type daemon struct {
	adMu          sync.Mutex
	advertisement *zeroconf.Server
	vowlanAd      *zeroconf.Server // 常驻 _airsim-vowlan 广告；与配对用的临时广告分开
	ifname        string
	mu            sync.Mutex
	session       *pairing.Session
	bridge        audio.Bridge
	lan           string
	deviceID      string
	client        *http.Client
	paired        bool
	callMu        sync.Mutex
	gateway       *gateway.Gateway
	settings      managementSettings
	directHosts   map[string]bool
	tunnel        string
	direct        *directListeners
	listenersMu   sync.Mutex
	// 重绑时要能给新地址重新挂上 HTTP/PCM 服务，这些句柄必须留在 daemon 上。
	ctx     context.Context
	cancel  context.CancelFunc
	public  http.Handler
	srvMu   sync.Mutex
	servers []*http.Server
}

func main() {
	iface := flag.String("interface", "", "legacy trusted LAN interface override")
	data := flag.String("data", "/mnt/mu300-disk/extra/airsim/data", "persistent private state")
	manage := flag.String("manage", "", "rpcd management command")
	monitorUpdates := flag.Bool("update-monitor", false, "periodically check published plugin versions")
	updateWorker := flag.String("update-worker", "", "internal detached upgrade worker")
	runAgent := flag.Bool("run-agent", false, "supervise Agent with MU300 management policy")
	selfTest := flag.Duration("audio-self-test", 0, "explicit no-call DSP self-test, 1s..30s")
	trial := flag.Bool("early-media-trial", false, "explicit 10-minute lab window; separately arm one current call for 60s")
	trialArm := flag.Bool("early-media-trial-arm", false, "locally authorize only the current owned outgoing call for receive-only trial")
	trialRevoke := flag.Bool("early-media-trial-revoke", false, "remove the local early-media trial permission")
	flag.Parse()
	if *trialArm || *trialRevoke {
		var err error
		if *trialArm && *trialRevoke {
			log.Fatal("choose arm or revoke")
		}
		if *trialArm {
			err = armEarlyMediaTrial(*data)
		} else {
			err = os.Remove(filepath.Join(*data, "early-media-trial.json"))
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			log.Fatal("early-media trial command failed: ", err)
		}
		log.Print("early-media trial permission updated; no call control or recording performed")
		return
	}
	if *updateWorker != "" {
		if installUpdate(*data, *updateWorker) != nil {
			os.Exit(1)
		}
		return
	}
	if *monitorUpdates {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer cancel()
		updateMonitor(ctx, *data)
		return
	}
	if *manage != "" {
		json.NewEncoder(os.Stdout).Encode(managementCommand(*manage, *data))
		return
	}
	if *runAgent {
		if e := startManagedAgent(*data); e != nil {
			log.Fatal("managed Agent stopped")
		}
		return
	}
	log.SetFlags(log.LstdFlags | log.LUTC)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	if *selfTest != 0 {
		b := &audio.Bridge{}
		if e := b.SelfTest(ctx, *selfTest); e != nil {
			log.Fatal(e)
		}
		json.NewEncoder(os.Stdout).Encode(b.Status())
		return
	}
	lock, e := managementLock(*data, false)
	if e != nil {
		log.Fatal(e)
	}
	defer lock.Close()
	settings, e := loadSettings(*data)
	if e != nil {
		log.Fatal(e)
	}
	if *iface != "" {
		settings.Interface = *iface
	}
	if e := settings.validate(); e != nil {
		log.Fatal(e)
	}
	if err := os.MkdirAll(*data, 0700); err != nil {
		log.Fatal(err)
	}
	id, e := identity(filepath.Join(*data, "agent-id"))
	if e != nil {
		log.Fatal(e)
	}
	lan, e := lanAddress(settings.Interface)
	if e != nil {
		log.Fatal(e)
	}
	d := &daemon{lan: lan, ifname: settings.Interface, settings: settings, deviceID: id, client: &http.Client{Timeout: 10 * time.Second}}
	d.gateway, e = gateway.New(filepath.Join(*data, "vowlan-clients.json"), id)
	if e != nil {
		log.Fatal(e)
	}
	d.gateway.SetEarlyMediaBackend(gateway.NewMU300EarlyMediaBackend(d.bridge.OpenEarlyMedia))
	if *trial {
		d.gateway.SetEarlyMediaBackend(gateway.NewMU300EarlyMediaTrialBackend(d.bridge.OpenEarlyMedia, filepath.Join(*data, "early-media-trial.json")))
	}
	listener, e := net.Listen("tcp", "127.0.0.1:7580")
	if e != nil {
		log.Fatal(e)
	}
	defer listener.Close()
	var sessions sync.WaitGroup
	acceptedDone := make(chan struct{})
	go func() { <-ctx.Done(); listener.Close() }()
	go func() {
		defer close(acceptedDone)
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			sessions.Add(1)
			go func() { defer sessions.Done(); d.bridge.Handle(ctx, c) }()
		}
	}()
	admin := http.NewServeMux()
	admin.HandleFunc("/status", d.status)
	// Internal Agent transport only; never registered on the LAN control mux.
	admin.HandleFunc("/internal/cloud-early-media", d.gateway.ServeCloudEarlyMedia)
	admin.HandleFunc("/pair/start", d.startPair)
	admin.HandleFunc("/rebind", d.rebindHandler)
	admin.HandleFunc("/pair/cancel", d.cancelPair)
	admin.HandleFunc("/number/refresh", d.refreshNumber)
	admin.HandleFunc("/call/action", d.callAction)
	public := http.NewServeMux()
	public.HandleFunc("/pair/v1/info", d.pairInfo)
	public.HandleFunc("/pair/v1/claim", d.claim)
	hosts, e := directAddresses(settings)
	if e != nil {
		log.Fatal(e)
	}
	if settings.TunnelInterface != "" {
		if ip, ok := interfaceAddress(settings.TunnelInterface, true); ok {
			d.tunnel = ip
		}
	}
	listeners, e := bindDirectListeners(hosts, settings)
	if e != nil {
		log.Fatal(e)
	}
	d.direct = listeners
	defer listeners.close()
	// 广告里的 host 必须是"手机当下真能连上的那个地址"：iOS 只认 TXT 的 host，
	// 不看 A 记录。切网由 airsim-netmode 写 /tmp/airsim-direct-host 并调 /rebind。
	if adHost := d.adHostFor(hosts); adHost != "" {
		d.lan = adHost
	}
	gwAd, e := d.publishAdvertisement(d.lan)
	if e != nil {
		log.Print("Bonjour unavailable; manual IP pairing remains available")
	} else {
		d.adMu.Lock()
		d.vowlanAd = gwAd
		d.adMu.Unlock()
		defer gwAd.Shutdown()
	}
	d.directHosts = make(map[string]bool)
	for _, host := range hosts {
		d.directHosts[host] = true
	}
	adminServer := &http.Server{Addr: "127.0.0.1:7581", Handler: admin, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 45 * time.Second, MaxHeaderBytes: 8192}
	d.ctx = ctx
	d.cancel = cancel
	d.public = privatePairing(public)
	d.srvMu.Lock()
	d.servers = append(d.servers, adminServer)
	d.srvMu.Unlock()
	go func() {
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			cancel()
		}
	}()
	for _, endpoint := range listeners.endpoints {
		d.serveEndpoint(endpoint)
	}
	log.Printf("MU300 plugin ready; pairing port %d", settings.PairPort)
	<-ctx.Done()
	d.closePair("")
	d.srvMu.Lock()
	for _, s := range d.servers {
		s.Close()
	}
	d.srvMu.Unlock()
	listener.Close()
	listeners.close()
	d.gateway.Shutdown()
	<-acceptedDone
	sessions.Wait()
}
func identity(path string) (string, error) {
	if b, e := os.ReadFile(path); e == nil && len(b) > 0 {
		return string(b), nil
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	id := "mu300-" + base64.RawURLEncoding.EncodeToString(b)
	return id, os.WriteFile(path, []byte(id), 0600)
}
func lanAddress(name string) (string, error) {
	i, e := net.InterfaceByName(name)
	if e != nil {
		return "", e
	}
	addrs, e := i.Addrs()
	if e != nil {
		return "", e
	}
	for _, a := range addrs {
		ip, _, _ := net.ParseCIDR(a.String())
		if ip.To4() != nil && ip.IsPrivate() {
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("no private IPv4 on %s", name)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func method(w http.ResponseWriter, r *http.Request, m string) bool {
	if r.Method != m {
		write(w, 405, map[string]string{"error": "method_not_allowed"})
		return false
	}
	return true
}
func (d *daemon) status(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	dev, e := audio.Discover()
	if e == nil {
		e = audio.CheckTools()
	}
	d.mu.Lock()
	var session any
	if d.session != nil && d.session.Expires > time.Now().UnixMilli() {
		session = map[string]any{"code": d.session.Code, "expires_at": d.session.Expires}
	}
	paired := d.paired
	d.mu.Unlock()
	d.adMu.Lock()
	bonjour := d.advertisement != nil
	d.adMu.Unlock()
	calls := d.callsSnapshot(r.Context())
	write(w, 200, map[string]any{"product": "AirSIM for MU300", "version": version, "running": true, "lan_ip": d.lan, "ad_host": d.lan, "bind_hosts": d.hostList(), "tunnel_ip": d.tunnel, "pairing_port": d.settings.PairPort, "control_port": d.settings.ControlPort, "pcm_port": d.settings.PCMPort, "mode": d.settings.Mode, "bonjour_pairing": session != nil && bonjour, "agent_id": d.deviceID, "pairing": session, "paired_this_boot": paired, "audio": d.bridge.Status(), "gateway": d.gateway.Diagnostics(), "devices": dev, "audio_available": e == nil, "agent": d.agentHealth(), "sim": d.agentGet("/api/status"), "relay": d.agentGet("/api/push/status"), "call_active": !calls.Available || len(calls.Calls) > 0, "calls": calls})
}

func (d *daemon) callActive() bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8575/api/calls/status", nil)
	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var state struct {
		Active *json.RawMessage `json:"active"`
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32768)).Decode(&state) == nil && state.Active != nil
}

func (d *daemon) refreshNumber(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "http://127.0.0.1:8575/api/sim/identity", nil)
	resp, err := d.client.Do(req)
	if err != nil {
		write(w, 502, map[string]string{"error": "无法连接 Agent，请确认服务已启动"})
		return
	}
	defer resp.Body.Close()
	var identity map[string]any
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&identity) != nil {
		write(w, 200, map[string]string{"error": "号码读取失败，请稍后重试"})
		return
	}
	write(w, 200, map[string]any{"ok": true, "phone_number": identity["phone_number"]})
}
func (d *daemon) agentGet(path string) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8575"+path, nil)
	resp, e := d.client.Do(req)
	if e != nil {
		return map[string]any{}
	}
	defer resp.Body.Close()
	var data map[string]any
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&data) != nil {
		return map[string]any{}
	}
	// Only return selected status and this SIM's own number; never credentials.
	result := map[string]any{}
	for _, key := range []string{"sim_inserted", "signal_dbm", "reg_status_text", "phone_number", "phone_number_status", "phone_number_updated_at", "configured", "cloud_enabled", "command_connected", "management_blocked", "call_push_ready", "last_error", "relay_url"} {
		if value, ok := data[key]; ok {
			result[key] = value
		}
	}
	return result
}
func (d *daemon) agentHealth() any {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8575/api/health", nil)
	resp, e := d.client.Do(r)
	if e != nil {
		return map[string]any{"reachable": false}
	}
	defer resp.Body.Close()
	var v map[string]any
	json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&v)
	return map[string]any{"reachable": resp.StatusCode == 200, "runtime_profile": v["runtime_profile"], "version": v["version"], "voice_backend": v["voice_backend"]}
}
func (d *daemon) startPair(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	s, e := pairing.New(time.Now())
	if e != nil {
		write(w, 500, map[string]string{"error": "pairing_creation_failed"})
		return
	}
	d.adMu.Lock()
	var ad *zeroconf.Server
	// 配对广告必须发在"手机所在那张网卡"上：Wi-Fi 上行时那是 wlan0，
	// 死写 br-lan 的话手机连组播都收不到（这也是"Wi-Fi 下搜不到配对"的一半原因）。
	if i := d.interfaceForHost(d.lan); i != nil {
		ad, _ = zeroconf.RegisterProxy("AirSIM-MU300-"+s.ID[:8], "_airsim-pair._tcp", "local.", d.settings.PairPort, "airsim-"+s.ID[:8], []string{d.lan}, []string{"v=1", "session=" + s.ID, "key=" + s.Public, fmt.Sprintf("expires=%d", s.Expires)}, []net.Interface{*i})
		if ad != nil {
			ad.TTL(30)
		}
	}
	if d.advertisement != nil {
		d.advertisement.Shutdown()
	}
	d.advertisement = ad
	d.mu.Lock()
	d.session = s
	d.mu.Unlock()
	d.adMu.Unlock()
	time.AfterFunc(120*time.Second, func() { d.closePair(s.ID) })
	write(w, 200, map[string]any{"code": s.Code, "expires_at": s.Expires, "host": d.lan, "port": d.settings.PairPort})
}
func (d *daemon) cancelPair(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	d.closePair("")
	write(w, 200, map[string]bool{"ok": true})
}
func (d *daemon) closePair(id string) {
	d.adMu.Lock()
	defer d.adMu.Unlock()
	d.mu.Lock()
	if id != "" && (d.session == nil || d.session.ID != id) {
		d.mu.Unlock()
		return
	}
	d.session = nil
	d.mu.Unlock()
	if d.advertisement != nil {
		d.advertisement.Shutdown()
		d.advertisement = nil
	}
}
func (d *daemon) activeSession() *pairing.Session {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.session == nil || d.session.Expires <= time.Now().UnixMilli() {
		return nil
	}
	return d.session
}
func (d *daemon) pairInfo(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "GET") {
		return
	}
	s := d.activeSession()
	if s == nil {
		write(w, 410, map[string]string{"error": "pairing_expired"})
		return
	}
	write(w, 200, s.Info())
}
func (d *daemon) claim(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	s := d.activeSession()
	if s == nil {
		write(w, 410, map[string]string{"error": "pairing_expired"})
		return
	}
	var claim pairing.Claim
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&claim) != nil {
		write(w, 400, map[string]string{"error": "invalid_claim"})
		return
	}
	if claim.WantDirect && !d.hasDirectRoute(r) {
		write(w, 503, map[string]string{"error": "direct_gateway_unavailable"})
		return
	}
	plaintext, e := s.Open(claim, time.Now())
	if e != nil {
		write(w, 401, map[string]string{"error": "pairing_auth_failed"})
		return
	}
	defer clear(plaintext)
	var reg map[string]any
	if json.Unmarshal(plaintext, &reg) != nil {
		write(w, 400, map[string]string{"error": "invalid_registration"})
		return
	}
	localSecret, _ := reg["vowlan_secret"].(string)
	if d.gateway == nil || d.gateway.CheckAdd(localSecret) != nil {
		write(w, 500, map[string]string{"error": "gateway_key_storage_failed"})
		return
	}
	result := map[string]any{"configured": true, "local_mode": true}
	pushConfigured := false
	localMode, _ := reg["local_mode"].(bool)
	if localMode {
		if enabled, _ := reg["cloud_enabled"].(bool); enabled {
			write(w, 400, map[string]string{"error": "local_mode_cloud_conflict"})
			return
		}
	} else {
		result = nil
		prepareRegistration(reg, d.deviceID)
		body, _ := json.Marshal(reg)
		defer clear(body)
		req, _ := http.NewRequestWithContext(r.Context(), "POST", "http://127.0.0.1:8575/api/push/register", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, e := d.client.Do(req)
		if e != nil {
			write(w, 502, map[string]string{"error": "agent_unavailable"})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			write(w, 502, map[string]string{"error": "agent_registration_rejected"})
			return
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 32768)).Decode(&result) != nil {
			write(w, 502, map[string]string{"error": "invalid_agent_response"})
			return
		}
		var ok bool
		pushConfigured, ok = result["configured"].(bool)
		if !ok {
			write(w, 502, map[string]string{"error": "invalid_agent_response"})
			return
		}
	}
	if d.gateway.Add(localSecret) != nil {
		write(w, 500, map[string]string{"error": "gateway_key_storage_failed"})
		return
	}
	// Agent HTTP 200 acknowledges validation and persistence. Its configured
	// field describes cloud push readiness, which can be false for a valid
	// pairing (disabled cloud, missing relay, or management policy). The phone
	// interprets configured on this endpoint as the pairing acknowledgement.
	// Keep readiness separate; never change the stored cloud preferences.
	result["push_configured"] = pushConfigured
	result["configured"] = true
	result["agent_id"] = d.deviceID
	if claim.WantDirect {
		key, _ := base64.RawURLEncoding.DecodeString(localSecret)
		result["control_port"] = d.settings.ControlPort
		result["pcm_port"] = d.settings.PCMPort
		result["route_proof"] = gateway.Sign(key, fmt.Sprintf("AIRSIM-DIRECT1\n%s\n%s\n%s\n%d\n%d", s.ID, claim.ClientPublic, d.deviceID, d.settings.ControlPort, d.settings.PCMPort))
		clear(key)
	}
	d.mu.Lock()
	d.paired = true
	d.mu.Unlock()
	d.closePair(s.ID)
	write(w, 200, result)
}

// These fields belong to the phone-side gateway, not the Go Agent's strict
// pushRegistration schema. MU300 has no Android notification-forwarding source.
// Preserve all cloud credentials; never log the decrypted registration.
func prepareRegistration(reg map[string]any, deviceID string) {
	delete(reg, "vowlan_secret")
	delete(reg, "notification_forwarding")
	reg["android_agent_id"] = deviceID
	reg["android_agent_kind"] = "mu300"
	reg["android_device_name"] = "AirSIM MU300"
	reg["android_manufacturer"] = "ZTE"
	reg["android_model"] = "MU300"
}

// ---- 直连监听与 Bonjour 广告的运行期重建 ----
// iOS 只认 TXT 里的 host（不看 A 记录），并且把配对时的地址存进 UserDefaults。
// 切网或换 IP 之后，设备必须把监听搬到新地址、把广告里的 host 换成新地址：
// 否则手机拿旧地址连不上，新地址又发现不了。这里做运行期重建而不是重启进程，
// 因为重启会把正在通话的控制 / PCM 会话一起掐掉。

const directHostFile = "/tmp/airsim-direct-host"

// serveEndpoint 给一组直连监听挂上配对 / 控制 / PCM 服务。
// 首次启动和运行期重绑共用这一个函数：重绑只搬地址、不重启进程，
// 但新 listener 必须重新挂服务，否则端口在听、却没人 accept（真机踩过：
// TCP 握手成功、HTTP 一直挂着超时）。
func (d *daemon) serveEndpoint(ep directEndpoint) {
	pairServer := &http.Server{Handler: d.public, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, MaxHeaderBytes: 8192}
	controlServer := &http.Server{Handler: d.gateway, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 80 * time.Second, MaxHeaderBytes: 8192}
	d.srvMu.Lock()
	d.servers = append(d.servers, pairServer, controlServer)
	d.srvMu.Unlock()
	go func() {
		if err := pairServer.Serve(ep.pair); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			d.cancel()
		}
	}()
	go func() {
		if err := controlServer.Serve(ep.control); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			d.cancel()
		}
	}()
	go func(l net.Listener) {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go d.gateway.ServePCM(d.ctx, c)
		}
	}(ep.pcm)
}

func (d *daemon) publishAdvertisement(host string) (*zeroconf.Server, error) {
	txt := []string{"v=1", "agent_id=" + d.deviceID, "agent_kind=mu300", "device_name=AirSIM MU300",
		"manufacturer=ZTE", "model=MU300", "host=" + host,
		"control_port=" + portText(d.settings.ControlPort),
		"pcm_port=" + portText(d.settings.PCMPort), "caps=control,pcm"}
	// 一条广告只能带一个 host：必须在 host 所属那张网卡上广播。
	// 同时往 br-lan 和 wlan0 发，另一侧手机就会拿到它根本去不了的地址。
	var ifaces []net.Interface
	if i := d.interfaceForHost(host); i != nil {
		ifaces = []net.Interface{*i}
	}
	name := "AirSIM-MU300"
	if id := d.deviceID; len(id) > 8 {
		name += "-" + id[len(id)-8:]
	}
	ad, err := zeroconf.Register(name, "_airsim-vowlan._tcp", "local.", d.settings.ControlPort, txt, ifaces)
	if err != nil {
		return nil, err
	}
	return ad, nil
}

func (d *daemon) interfaceForHost(host string) *net.Interface {
	for _, n := range directInterfaceNames(d.settings) {
		if ip, ok := interfaceAddress(n, n == d.settings.TunnelInterface); ok && ip == host {
			if i, err := net.InterfaceByName(n); err == nil {
				return i
			}
		}
	}
	if i, err := net.InterfaceByName(d.settings.Interface); err == nil {
		return i
	}
	return nil
}

// adHostFor 选广告 host：优先采纳 netmode 写的指示文件（它知道当下是热点还是
// Wi-Fi 上行），但文件里的地址必须真在当前监听列表里，防止写过时的值。
func (d *daemon) adHostFor(hosts []string) string {
	if b, err := os.ReadFile(directHostFile); err == nil {
		if h := strings.TrimSpace(string(b)); h != "" {
			for _, v := range hosts {
				if v == h {
					return h
				}
			}
		}
	}
	// 指示文件可能是上一次开机残留的旧地址（已经不在监听列表里）。
	// 这种情况按 /proc/net/route 判断当下哪张卡扛着默认路由：Wi-Fi 模式是
	// 无线口，热点/蜂窝模式是 LAN 桥。重启后也能自己选对。
	if s := d.settings.STAInterface; s != "" && interfaceDefaultRoute(s) {
		if ip, ok := interfaceAddress(s, false); ok {
			return ip
		}
	}
	if ip, ok := interfaceAddress(d.settings.Interface, false); ok {
		return ip
	}
	return d.lan
}

// interfaceDefaultRoute 看某网卡是否扛着默认路由（/proc/net/route 里
// destination 全 0 的那一行）。
func interfaceDefaultRoute(name string) bool {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if f[0] == name && f[1] == "00000000" {
			return true
		}
	}
	return false
}

func (d *daemon) hostListLocked() []string {
	hosts := make([]string, 0, len(d.directHosts))
	for h := range d.directHosts {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	return hosts
}

func (d *daemon) hostList() []string {
	d.listenersMu.Lock()
	defer d.listenersMu.Unlock()
	return d.hostListLocked()
}

func (d *daemon) rebind() error {
	d.listenersMu.Lock()
	defer d.listenersMu.Unlock()
	hosts, err := directAddresses(d.settings)
	if err != nil {
		return err
	}
	// 必须先关旧的再绑新的：地址没变时同一进程再绑同一 host:port 会直接
	// EADDRINUSE（2026-11-12 真机踩到）。代价是中间有一毫秒级空窗，可接受。
	oldHosts := d.hostListLocked()
	if old := d.direct; old != nil {
		old.close()
	}
	d.direct = nil
	next, err := bindDirectListeners(hosts, d.settings)
	if err != nil {
		// 新地址绑不上就退回旧地址，别让设备变成"哪边都不通"
		if prev, e2 := bindDirectListeners(oldHosts, d.settings); e2 == nil {
			d.direct = prev
		}
		return err
	}
	d.direct = next
	for _, ep := range next.endpoints {
		d.serveEndpoint(ep)
	}
	dm := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		dm[h] = true
	}
	d.directHosts = dm
	if d.settings.TunnelInterface != "" {
		if ip, ok := interfaceAddress(d.settings.TunnelInterface, true); ok {
			d.tunnel = ip
		} else {
			d.tunnel = ""
		}
	}
	adHost := d.adHostFor(hosts)
	ad, adErr := d.publishAdvertisement(adHost)
	d.adMu.Lock()
	old := d.vowlanAd
	if adErr == nil {
		d.vowlanAd = ad
	}
	d.adMu.Unlock()
	if old != nil {
		old.Shutdown()
	}
	d.mu.Lock()
	d.lan = adHost
	d.mu.Unlock()
	// 新地址的监听已经起来，广告发不发得出去属于次要问题：
	// mDNS 挂了还有手动填地址这条路，不该让整次重绑算失败。
	return nil
}

func (d *daemon) rebindHandler(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, "POST") {
		return
	}
	if err := d.rebind(); err != nil {
		write(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	write(w, 200, map[string]any{"ok": true, "ad_host": d.lan, "bind_hosts": d.hostList(), "tunnel_ip": d.tunnel})
}
