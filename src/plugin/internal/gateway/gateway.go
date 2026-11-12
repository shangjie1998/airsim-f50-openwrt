// Package gateway implements the existing AirSIM VoWLAN wire protocol.
// The Agent and unencrypted DSP socket remain bound to loopback.
package gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxGatewayKeys 是密钥表上限。满了不再报错，改为淘汰最久没被用过的一条。
const maxGatewayKeys = 16

// keyInfo 是每条 secret 的旁挂信息，存在同目录的 .meta 文件里。
// vowlan-clients.json 本身仍是纯 secret 数组，老设备、已配对手机都不受影响；
// .meta 丢了或坏了只是退回"先到先得"，鉴权照常。
type keyInfo struct {
	Client string `json:"client,omitempty"` // 客户端公钥（配对 claim 里的 client_public）
	Added  int64  `json:"added,omitempty"`
	Used   int64  `json:"used,omitempty"`
}

type Gateway struct {
	mu              sync.Mutex
	path, id        string
	keys            []string
	meta            map[string]keyInfo
	seen            map[string]int64
	pcm             bool
	earlyBackend    EarlyMediaBackend
	earlyLeases     map[string]*earlyLease
	earlyStreamDone chan struct{}
	pcmSessions     sync.WaitGroup
	closing         bool
	client          *http.Client
	events          map[string]uint64
	lastEvent       string
	lastEventAt     string
}

func New(path, id string) (*Gateway, error) {
	g := &Gateway{path: path, id: id, seen: make(map[string]int64), meta: make(map[string]keyInfo), client: &http.Client{Timeout: 75 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	// 元数据单独一个文件，读不到就算了，不能因为它坏了把整个服务拦住。
	if mb, err := os.ReadFile(path + ".meta"); err == nil {
		var meta map[string]keyInfo
		if json.Unmarshal(mb, &meta) == nil && meta != nil {
			g.meta = meta
		}
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return g, nil
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(b, &g.keys) != nil || len(g.keys) > maxGatewayKeys {
		return nil, errors.New("invalid gateway state")
	}
	for _, key := range g.keys {
		if _, err := decode(key); err != nil {
			return nil, err
		}
	}
	// 老版本的密钥表没有旁挂信息。给它们补一个以文件修改时间为准的基准，
	// 免得一上来就被当成"从没被用过"而在淘汰时最先出局。
	if fi, err := os.Stat(path); err == nil {
		base := fi.ModTime().Unix()
		for _, key := range g.keys {
			if info, ok := g.meta[key]; !ok || info.Added == 0 {
				info.Added = base
				if info.Used == 0 {
					info.Used = base
				}
				g.meta[key] = info
			}
		}
	}
	return g, nil
}
func decode(s string) ([]byte, error) {
	b, e := base64.RawURLEncoding.DecodeString(s)
	if e != nil || len(b) != 32 {
		return nil, errors.New("invalid gateway key")
	}
	return b, nil
}

// CheckAdd checks a candidate without authorizing it. Pairing uses this before
// Agent registration; a rejected registration must not consume a client slot.
// Add checks again when committing, since capacity may have changed meanwhile.
func (g *Gateway) CheckAdd(encoded string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.checkAddLocked(encoded)
}
func (g *Gateway) checkAddLocked(encoded string) error {
	if _, err := decode(encoded); err != nil {
		return err
	}
	for _, key := range g.keys {
		if key == encoded {
			return nil
		}
	}
	// 满 16 条不再预检拒绝：Commit 时会按"最久未用"淘汰一条再收进来，
	// 否则反复配对第 17 次永远 500。
	return nil
}
func (g *Gateway) Add(encoded string) error { return g.AddClient(encoded, "") }

// AddClient 收一条密钥。client 是客户端公钥标识（配对 claim 的 client_public），
// 传空表示不知道身份，退回纯 secret 去重。
//   - 同一个 client 重新配对：旧 secret 被替换，不会一格一格往上叠
//   - 真满了 maxGatewayKeys 条：淘汰 Used 最小（最久没被用过）的那条
func (g *Gateway) AddClient(encoded, client string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.checkAddLocked(encoded); err != nil {
		return err
	}
	if g.meta == nil {
		g.meta = make(map[string]keyInfo)
	}
	now := time.Now().Unix()

	for _, key := range g.keys {
		if key == encoded {
			info := g.meta[key]
			if client != "" {
				info.Client = client
			}
			if info.Added == 0 {
				info.Added = now
			}
			info.Used = now
			g.meta[key] = info
			return g.persistMeta()
		}
	}

	keys := append([]string(nil), g.keys...)
	if client != "" {
		for i, key := range keys {
			if info, ok := g.meta[key]; ok && info.Client == client {
				keys = append(keys[:i], keys[i+1:]...)
				delete(g.meta, key)
				break
			}
		}
	}
	if len(keys) >= maxGatewayKeys {
		victim := 0
		for i := 1; i < len(keys); i++ {
			cur, best := g.meta[keys[i]], g.meta[keys[victim]]
			if cur.Used < best.Used || (cur.Used == best.Used && cur.Added < best.Added) {
				victim = i
			}
		}
		delete(g.meta, keys[victim])
		keys = append(keys[:victim], keys[victim+1:]...)
	}
	keys = append(keys, encoded)
	g.meta[encoded] = keyInfo{Client: client, Added: now, Used: now}

	if err := g.persist(keys); err != nil {
		return err
	}
	g.keys = keys
	return g.persistMeta()
}

// writeAtomic 写临时文件、fsync、再 rename，避免写一半断电留下半截 JSON。
func writeAtomic(path string, data []byte) error {
	f, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (g *Gateway) persist(keys []string) error {
	b, _ := json.Marshal(keys)
	return writeAtomic(g.path, b)
}

func (g *Gateway) persistMeta() error {
	b, _ := json.Marshal(g.meta)
	return writeAtomic(g.path+".meta", b)
}
func Sign(key []byte, canonical string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func canonical(method, path string, body []byte, ts, nonce string) string {
	hash := sha256.Sum256(body)
	return fmt.Sprintf("%s\n%s\n%x\n%s\n%s", method, path, hash, ts, nonce)
}
func (g *Gateway) authenticate(method, path string, body []byte, ts, nonce, sig string) []byte {
	stamp, err := strconv.ParseInt(ts, 10, 64)
	now := time.Now().Unix()
	if err != nil || stamp < now-30 || stamp > now+30 || len(nonce) < 7 || len(nonce) > 128 || len(sig) != 43 {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for n, expiry := range g.seen {
		if expiry < now {
			delete(g.seen, n)
		}
	}
	if _, ok := g.seen[nonce]; ok {
		return nil
	}
	if len(g.seen) >= 8192 {
		return nil
	}
	for _, encoded := range g.keys {
		key, _ := decode(encoded)
		if hmac.Equal([]byte(Sign(key, canonical(method, path, body, ts, nonce))), []byte(sig)) {
			g.seen[nonce] = now + 61
			// 只记在内存里，落盘等下次配对时顺带写，别为每个请求写磁盘
			if g.meta != nil {
				info := g.meta[encoded]
				info.Used = now
				if info.Added == 0 {
					info.Added = now
				}
				g.meta[encoded] = info
			}
			return key
		}
	}
	return nil
}
func allowed(method, path string) bool {
	if method == "PATCH" && path == "/api/sms/settings" {
		return true
	}
	if method == "GET" {
		switch path {
		case "/api/calls/early-media", "/api/sms", "/api/sms/status", "/v1/health", "/api/health", "/api/status", "/api/sim/identity", "/api/events", "/api/calls/status", "/api/calls/events", "/api/calls/operation", "/api/calls/audio/host/config", "/api/push/status":
			return true
		}
	}
	if method == "POST" {
		switch path {
		case "/api/calls/resolve", "/api/calls/history/ack", "/api/sms/send", "/api/sms/ack", "/api/sms/refresh", "/api/calls/dial", "/api/calls/answer", "/api/calls/reject", "/api/calls/hangup", "/api/calls/dtmf", "/api/calls/audio/mute", "/api/calls/audio/host/warmup", "/api/calls/audio/host/register":
			return true
		}
	}
	return false
}
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.event("control_request")
	w.Header().Set("Cache-Control", "no-store")
	if !allowed(r.Method, r.URL.Path) {
		http.Error(w, "not found", 404)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
	if err != nil {
		http.Error(w, "body too large", 413)
		return
	}
	h := r.Header
	nonce := h.Get("X-AirSIM-VoWLAN-Nonce")
	key := g.authenticate(r.Method, r.URL.RequestURI(), body, h.Get("X-AirSIM-VoWLAN-Timestamp"), nonce, h.Get("X-AirSIM-VoWLAN-Signature"))
	if h.Get("X-AirSIM-VoWLAN-Version") != "1" || key == nil {
		g.event("control_auth_rejected")
		http.Error(w, "authentication required", 401)
		return
	}
	defer clear(key)
	g.event("control_authenticated")
	if r.URL.Path == "/api/calls/early-media" {
		g.earlyMediaControl(w, r, key, nonce)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/api/calls/dial" {
		g.event("dial_request")
	}
	if r.URL.Path == "/v1/health" {
		w.Header().Set("Content-Type", "application/json")
		capable, number, battery := g.agentIdentity(r.Context())
		health := map[string]any{"ok": true, "version": 1, "transport": "vowlan", "agent_id": g.id, "identity_proof": Sign(key, "AIRSIM-HEALTH1\n"+nonce+"\n"+g.id), "canonical_call_ids": capable, "phone_number": number, "agent_kind": "mu300", "device_name": "AirSIM MU300", "manufacturer": "ZTE", "model": "MU300"}
		if battery != nil {
			health["battery"] = battery
		}
		json.NewEncoder(w).Encode(health)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, "http://127.0.0.1:8575"+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// Derive the trusted owner from the verified pairing key. Never copy a
	// caller-provided internal identity header into the loopback request.
	ownerDigest := sha256.Sum256(key)
	req.Header.Set("X-AirSIM-Internal-Call-Owner", fmt.Sprintf("wlan:%x", ownerDigest))
	resp, err := g.client.Do(req)
	if err != nil {
		http.Error(w, "agent unavailable", 502)
		return
	}
	defer resp.Body.Close()
	if r.URL.Path == "/api/calls/operation" && resp.StatusCode == http.StatusOK {
		output, readErr := io.ReadAll(io.LimitReader(resp.Body, 32769))
		if readErr != nil || len(output) > 32768 {
			http.Error(w, "invalid operation receipt", 502)
			return
		}
		output, err = signedOperationStatus(r.URL.Query(), output, nonce, key, g.id)
		if err != nil {
			http.Error(w, "unconfirmed operation receipt", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(output)
		return
	}
	if r.URL.Path == "/api/calls/dial" && resp.StatusCode == http.StatusOK {
		output, err := io.ReadAll(io.LimitReader(resp.Body, 32769))
		if err != nil || len(output) > 32768 {
			http.Error(w, "invalid dial receipt", 502)
			return
		}
		output, err = signedDialReceipt(body, output, nonce, key)
		if err != nil {
			http.Error(w, "unconfirmed dial identity; query call status before retrying", 502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(output)
		return
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(resp.StatusCode)
	// SSE events must reach clients immediately, not sit in net/http buffers.
	buf := make([]byte, 4096)
	for {
		n, e := resp.Body.Read(buf)
		if n > 0 {
			if _, err = w.Write(buf[:n]); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if e != nil {
			return
		}
	}
}

func (g *Gateway) agentIdentity(parent context.Context) (bool, string, *batteryReading) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8575/api/health", nil)
	resp, err := g.client.Do(req)
	if err != nil {
		return false, "", nil
	}
	defer resp.Body.Close()
	var health struct {
		Battery   json.RawMessage `json:"battery"`
		OK        bool            `json:"ok"`
		Canonical bool            `json:"canonical_call_ids"`
		Profile   string          `json:"runtime_profile"`
		Number    string          `json:"phone_number"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 32768)).Decode(&health) != nil || health.Profile != "mu300-openwrt" {
		return false, "", nil
	}
	return health.OK && health.Canonical, health.Number, validBattery(health.Battery, time.Now())
}

func signedDialReceipt(request, response []byte, nonce string, key []byte) ([]byte, error) {
	var asked struct {
		Operation string `json:"operation_id"`
	}
	if err := json.Unmarshal(request, &asked); err != nil {
		return nil, err
	}
	if asked.Operation == "" {
		return response, nil
	}
	var receipt map[string]any
	if err := json.Unmarshal(response, &receipt); err != nil {
		return nil, err
	}
	op, _ := receipt["operation_id"].(string)
	canonicalID, _ := receipt["android_id"].(string)
	backendID, _ := receipt["backend_id"].(string)
	if op != asked.Operation || canonicalID == "" || backendID == "" || strings.ContainsAny(op+canonicalID+backendID, "\r\n\x00") {
		return nil, errors.New("invalid dial identity")
	}
	receipt["operation_proof"] = Sign(key, "AIRSIM-DIAL1\n"+nonce+"\n"+op+"\n"+canonicalID+"\n"+backendID)
	return json.Marshal(receipt)
}
func (g *Gateway) ServePCM(ctx context.Context, c net.Conn) {
	g.mu.Lock()
	if g.closing {
		g.mu.Unlock()
		c.Close()
		return
	}
	g.pcmSessions.Add(1)
	g.mu.Unlock()
	defer g.pcmSessions.Done()
	g.event("pcm_connection")
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReaderSize(c, 1024)
	line, err := reader.ReadSlice('\n')
	if err != nil || len(line) > 1024 {
		g.event("pcm_header_rejected")
		return
	}
	parts := strings.Split(strings.TrimSuffix(string(line), "\n"), " ")
	if len(parts) > 0 && parts[0] == "AIRSIMEM1" {
		g.serveEarlyPCM(ctx, c, reader, parts)
		return
	}
	if len(parts) != 4 || parts[0] != "AIRSIMVWL1" {
		g.event("pcm_header_rejected")
		return
	}
	key := g.authenticate("PCM", "/v1/pcm", nil, parts[1], parts[2], parts[3])
	if key == nil {
		g.event("pcm_auth_rejected")
		return
	}
	clear(key)
	g.event("pcm_authenticated")
	// Wait for receive-only teardown before opening duplex on the shared DSP.
	g.mu.Lock()
	earlyDone := g.earlyStreamDone
	g.mu.Unlock()
	if earlyDone != nil {
		select {
		case <-earlyDone:
		case <-ctx.Done():
			return
		case <-time.After(18 * time.Second):
			return
		}
	}
	g.mu.Lock()
	if g.pcm {
		g.mu.Unlock()
		g.event("pcm_busy")
		return
	}
	g.pcm = true
	g.mu.Unlock()
	defer func() { g.mu.Lock(); g.pcm = false; g.mu.Unlock() }()
	upstream, err := net.DialTimeout("tcp", "127.0.0.1:7580", 3*time.Second)
	if err != nil {
		g.event("pcm_backend_unavailable")
		return
	}
	defer upstream.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
			upstream.Close()
		case <-done:
		}
	}()
	upstream.SetDeadline(time.Now().Add(18 * time.Second))
	if _, err = io.WriteString(upstream, "AIRSIMPCM1\n"); err != nil {
		g.event("pcm_backend_handshake_failed")
		return
	}
	ack := make([]byte, len("AIRSIMREADY"))
	if _, err = io.ReadFull(upstream, ack); err != nil || string(ack) != "AIRSIMREADY" {
		g.event("pcm_backend_not_ready")
		return
	}
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = c.Write(ack); err != nil {
		g.event("pcm_client_disconnected_before_ready")
		return
	}
	c.SetDeadline(time.Time{})
	upstream.SetDeadline(time.Time{})
	g.event("pcm_ready")
	copied := make(chan struct{})
	go func() { io.Copy(upstream, reader); upstream.Close(); c.Close(); close(copied) }()
	io.Copy(c, upstream)
	upstream.Close()
	c.Close()
	<-copied
	g.event("pcm_closed")
}

// Shutdown follows cancellation of the server context. Stop admission under the
// same lock as Add, then wait for receive-only capture/DSP teardown as well as
// ordinary PCM sessions before the daemon may exit.
func (g *Gateway) Shutdown() {
	g.mu.Lock()
	g.closing = true
	g.mu.Unlock()
	g.pcmSessions.Wait()
}

// clientOnlineWindow 是一把密钥多久没鉴权就算"离线"。客户端醒着时每几秒
// 就来查一次 health / 长轮询 events，所以两分钟没有动静就是走了。
const clientOnlineWindow = 120

// ClientInfo 是页面能看到的已配对设备信息。密钥本身不透明，只暴露尾号区分。
type ClientInfo struct {
	Index       int    `json:"index"`
	Fingerprint string `json:"fingerprint"`
	LastUsed    int64  `json:"last_used,omitempty"`
	Online      bool   `json:"online"`
}

// fingerprintOf 取密钥编码的尾 9 位。
// 必须和 rpcd 回退路径里的 `tail -c 9` 一致，否则同一条密钥在两条数据
// 来源下会显示成两个不同的尾号，用户会以为多出来一台设备。
func fingerprintOf(encoded string) string {
	const n = 9
	if len(encoded) <= n {
		return encoded
	}
	return encoded[len(encoded)-n:]
}

// Clients 按文件顺序列出已配对设备，附带各自最后一次鉴权时间和在线状态。
// 时间戳取自 meta 里鉴权命中时记的 Used，只用于展示，不参与鉴权。
func (g *Gateway) Clients() []ClientInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().Unix()
	out := make([]ClientInfo, 0, len(g.keys))
	for i, encoded := range g.keys {
		last := g.meta[encoded].Used
		out = append(out, ClientInfo{
			Index:       i + 1,
			Fingerprint: fingerprintOf(encoded),
			LastUsed:    last,
			Online:      last > 0 && now-last <= clientOnlineWindow,
		})
	}
	return out
}
