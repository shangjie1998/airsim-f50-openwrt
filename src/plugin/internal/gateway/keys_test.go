package gateway

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// testKey 造一条合法的 32 字节 secret（base64url）。
func testKey(seed int) string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(seed*7 + i)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func newTestGateway(t *testing.T) *Gateway {
	t.Helper()
	g, err := New(filepath.Join(t.TempDir(), "vowlan-clients.json"), "mu300-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g
}

// 同一台手机重新配对：拿新 secret 换掉旧的那条，密钥表不该涨。
func TestAddClientReplacesSameClient(t *testing.T) {
	g := newTestGateway(t)
	oldSecret, newSecret := testKey(1), testKey(2)

	if err := g.AddClient(oldSecret, "client-A"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := g.AddClient(newSecret, "client-A"); err != nil {
		t.Fatalf("second add: %v", err)
	}
	if len(g.keys) != 1 {
		t.Fatalf("want 1 key after re-pair, got %d: %v", len(g.keys), g.keys)
	}
	if g.keys[0] != newSecret {
		t.Fatalf("want the new secret to replace the old one, got %q", g.keys[0])
	}
	if _, ok := g.meta[oldSecret]; ok {
		t.Fatalf("old secret should be dropped from meta")
	}
	if g.meta[newSecret].Client != "client-A" {
		t.Fatalf("meta should carry the client id")
	}
}

// 满 16 条之后再配第 17 次：淘汰最久没被用过的那条，而不是 500。
func TestAddClientEvictsLeastRecentlyUsed(t *testing.T) {
	g := newTestGateway(t)
	secrets := make([]string, maxGatewayKeys)
	for i := 0; i < maxGatewayKeys; i++ {
		secrets[i] = testKey(i + 10)
		if err := g.AddClient(secrets[i], ""); err != nil {
			t.Fatalf("add #%d: %v", i, err)
		}
	}
	if len(g.keys) != maxGatewayKeys {
		t.Fatalf("want %d keys, got %d", maxGatewayKeys, len(g.keys))
	}

	// 人为把第 3 条标成"很久以前用过"，其余都刚用过
	for i, s := range secrets {
		info := g.meta[s]
		info.Used = 2000
		if i == 3 {
			info.Used = 100
		}
		g.meta[s] = info
	}

	fresh := testKey(99)
	if err := g.AddClient(fresh, "client-new"); err != nil {
		t.Fatalf("add when full: %v", err)
	}
	if len(g.keys) != maxGatewayKeys {
		t.Fatalf("want table still at %d after eviction, got %d", maxGatewayKeys, len(g.keys))
	}
	for _, s := range g.keys {
		if s == secrets[3] {
			t.Fatalf("least recently used key should have been evicted")
		}
	}
	if g.keys[len(g.keys)-1] != fresh {
		t.Fatalf("new key should be appended")
	}
}

// 旧版本只写纯 secret 数组，没有 .meta；加载后鉴权照旧，新增一条也不该炸。
func TestLegacyKeysFileStillLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vowlan-clients.json")
	legacy := []string{testKey(1), testKey(2)}
	b, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatalf("seed legacy file: %v", err)
	}

	g, err := New(path, "mu300-test")
	if err != nil {
		t.Fatalf("New on legacy file: %v", err)
	}
	if len(g.keys) != 2 {
		t.Fatalf("want 2 legacy keys, got %d", len(g.keys))
	}
	if g.meta == nil {
		t.Fatalf("meta map should be initialized even without a .meta file")
	}

	if err := g.AddClient(testKey(3), "client-A"); err != nil {
		t.Fatalf("add on legacy table: %v", err)
	}
	if len(g.keys) != 3 {
		t.Fatalf("want 3 keys, got %d", len(g.keys))
	}

	// 文件仍是纯数组，别把老格式改成对象
	var reread []string
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &reread); err != nil {
		t.Fatalf("keys file must stay a plain string array: %v", err)
	}
	if len(reread) != 3 {
		t.Fatalf("persisted keys = %d, want 3", len(reread))
	}
	// 元数据写到旁挂文件里
	if _, err := os.Stat(path + ".meta"); err != nil {
		t.Fatalf("meta file should exist after add: %v", err)
	}
}

// 满了以后 CheckAdd 不能再报"额度用尽"，否则配对在预检阶段就被打回。
func TestCheckAddAcceptsWhenFull(t *testing.T) {
	g := newTestGateway(t)
	for i := 0; i < maxGatewayKeys; i++ {
		if err := g.AddClient(testKey(i+10), ""); err != nil {
			t.Fatalf("seed #%d: %v", i, err)
		}
	}
	if err := g.CheckAdd(testKey(77)); err != nil {
		t.Fatalf("CheckAdd on a full table should pass: %v", err)
	}
	// 但非法 secret 仍然要拒
	if err := g.CheckAdd("not-base64"); err == nil {
		t.Fatalf("malformed secret must still be rejected")
	}
}
