package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const mu300OperationLimit = 4096
const mu300OwnerHeader = "X-AirSIM-Internal-Call-Owner"

type mu300Operation struct {
	Parameters  string     `json:"parameters_digest"`
	OwnerDigest string     `json:"owner_digest,omitempty"`
	Status      string     `json:"status"`
	CallID      string     `json:"call_id,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}
type mu300OperationJournal struct {
	Version int                       `json:"version"`
	Records map[string]mu300Operation `json:"records"`
}

// No eviction: an old intent must never become executable again. A full or
// invalid journal fails closed. Only digests, outcomes and call UUIDs persist.
type mu300OperationStore struct {
	mu        sync.Mutex
	directory string
	loaded    bool
	journal   mu300OperationJournal
	fault     error
}

func mu300ValidDigest(s string) bool { b, e := hex.DecodeString(s); return e == nil && len(b) == 32 }
func mu300ValidIdentity(s string) bool {
	return s != "" && len(s) <= 128 && !strings.ContainsAny(s, "\r\n\x00")
}
func mu300OperationKey(owner, op string) string          { return mu300DialHash(owner + "\n" + op) }
func mu300OperationParameters(number, sim string) string { return mu300DialHash(number + "\n" + sim) }
func (s *mu300OperationStore) load() error {
	if s.fault != nil {
		return s.fault
	}
	if s.loaded {
		return nil
	}
	data, err := readMU300PrivateFile(filepath.Join(s.directory, "operations.json"), 2<<20)
	if errors.Is(err, os.ErrNotExist) {
		s.journal = mu300OperationJournal{Version: 1, Records: map[string]mu300Operation{}}
		s.loaded = true
		return nil
	}
	if err == nil {
		err = json.Unmarshal(data, &s.journal)
	}
	valid := err == nil && s.journal.Version == 1 && s.journal.Records != nil && len(s.journal.Records) <= mu300OperationLimit
	for k, r := range s.journal.Records {
		valid = valid && mu300ValidDigest(k) && mu300ValidDigest(r.Parameters) && (r.OwnerDigest == "" || mu300ValidDigest(r.OwnerDigest))
		switch r.Status {
		case "unknown", "rejected":
			valid = valid && r.CallID == "" && r.EndedAt == nil
		case "confirmed":
			valid = valid && strings.HasPrefix(r.CallID, "mu300-") && mu300ValidIdentity(r.CallID)
		default:
			valid = false
		}
	}
	if !valid {
		s.fault = errors.New("拨号回执记录不可用，未发起新的电话")
		return s.fault
	}
	s.loaded = true
	return nil
}
func (s *mu300OperationStore) save() error {
	if err := (&mu300SMSBackend{state: s.directory}).save("operations.json", s.journal); err != nil {
		s.fault = errors.New("无法持久保存拨号回执，结果可能未知；请查询状态，勿重复拨号")
		return s.fault
	}
	return nil
}
func (s *mu300OperationStore) begin(owner, op, number, sim string) (mu300Operation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return mu300Operation{}, false, err
	}
	key, parameters := mu300OperationKey(owner, op), mu300OperationParameters(number, sim)
	if r, ok := s.journal.Records[key]; ok {
		if r.Parameters != parameters {
			return r, true, errors.New("拨号操作 ID 已用于其他参数，未执行")
		}
		return r, true, nil
	}
	if len(s.journal.Records) >= mu300OperationLimit {
		return mu300Operation{}, false, errors.New("持久拨号回执容量已满，未发起电话；重启不会清除记录")
	}
	r := mu300Operation{Parameters: parameters, OwnerDigest: mu300DialHash(owner), Status: "unknown"}
	s.journal.Records[key] = r
	return r, false, s.save()
}
func (s *mu300OperationStore) finish(owner, op, callID string, submitted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return err
	}
	key := mu300OperationKey(owner, op)
	r, ok := s.journal.Records[key]
	if !ok {
		return errors.New("拨号回执占位缺失")
	}
	if callID != "" {
		r.Status = "confirmed"
		r.CallID = callID
	} else if !submitted {
		r.Status = "rejected"
	}
	s.journal.Records[key] = r
	return s.save()
}
func (s *mu300OperationStore) lookup(owner, op string) (mu300Operation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		return mu300Operation{}, false, err
	}
	r, ok := s.journal.Records[mu300OperationKey(owner, op)]
	return r, ok, nil
}
func (s *mu300OperationStore) ended(calls []callRecord) {
	if len(calls) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.load() != nil {
		return
	}
	changed := false
	for key, r := range s.journal.Records {
		for _, c := range calls {
			if r.Status == "confirmed" && r.CallID == c.ID && r.EndedAt == nil && c.EndedAt != nil {
				end := *c.EndedAt
				r.EndedAt = &end
				s.journal.Records[key] = r
				changed = true
			}
		}
	}
	if changed {
		_ = s.save()
	}
}
func (r mu300Operation) receipt(op string) (map[string]any, error) {
	if r.Status == "unknown" {
		return nil, errors.New("此拨号操作已登记，结果尚未确认；未重复拨号，请查询操作状态")
	}
	if r.Status != "confirmed" {
		return nil, errors.New("此拨号操作已被拒绝，未重复执行；如需重试请明确发起新的操作")
	}
	result := map[string]any{"dialing": r.EndedAt == nil, "operation_id": op, "android_id": r.CallID, "backend_id": r.CallID}
	if r.EndedAt != nil {
		result["state"] = "ended"
		result["ended_at"] = r.EndedAt
	}
	return result, nil
}

// This header is set by our authenticated loopback gateway, never copied from
// caller-supplied HTTP headers. Direct local management has a separate scope.
func mu300RequestOwner(r *http.Request) (string, error) {
	owner := r.Header.Get(mu300OwnerHeader)
	if owner == "" {
		return "local", nil
	}
	if !strings.HasPrefix(owner, "wlan:") || !mu300ValidDigest(strings.TrimPrefix(owner, "wlan:")) {
		return "", errors.New("无效拨号请求身份")
	}
	return owner, nil
}
func (a *agent) mu300OperationStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	if a.profile.Name != "mu300-openwrt" || a.mu300Operations == nil {
		writeError(w, 404, "操作状态查询不可用")
		return
	}
	owner, err := mu300RequestOwner(r)
	op := r.URL.Query().Get("operation_id")
	if err != nil || !mu300ValidIdentity(op) || len(r.URL.Query()["operation_id"]) != 1 {
		writeError(w, 400, "无效操作查询")
		return
	}
	record, found, err := a.mu300Operations.lookup(owner, op)
	if err != nil {
		writeError(w, 503, "拨号回执记录不可用")
		return
	}
	if !found {
		writeError(w, 404, "未找到此身份的操作记录")
		return
	}
	result := map[string]any{"version": 1, "operation_id": op, "status": record.Status, "call_state": "unknown"}
	if record.CallID != "" {
		result["android_id"] = record.CallID
		result["backend_id"] = record.CallID
	}
	if record.EndedAt != nil {
		result["call_state"] = "ended"
		result["ended_at"] = record.EndedAt
	} else if record.CallID != "" {
		a.mu.RLock()
		for _, c := range a.mu300Calls {
			if c.ID == record.CallID {
				result["call_state"] = c.State
			}
		}
		a.mu.RUnlock()
	}
	writeJSON(w, 200, result)
}
