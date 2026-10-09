package nativeengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	JournalVersion    = 1
	MaxJournalEntries = 2048
	MaxJournalBytes   = 2 << 20
)

// Journal只保存冻结启动请求、已提交输入和每次原生回复的校验值。
// 调用方必须将候选日志和投递、成绩放在同一个数据库事务内提交。
// 原生进程缓存可以随时丢弃，绝不能作为第二份持久战斗权威。
type Journal struct {
	Version      int             `json:"version"`
	ResourceHash string          `json:"resource_hash"`
	Start        json.RawMessage `json:"start"`
	StartDigest  string          `json:"start_digest"`
	Entries      []JournalEntry  `json:"entries,omitempty"`
	Head         string          `json:"head"`
}

type JournalEntry struct {
	Request     json.RawMessage `json:"request"`
	ReplyDigest string          `json:"reply_digest"`
	Head        string          `json:"head"`
}

// Authority在一个缓存上串行运行。不同服务进程从同一份已提交日志重建并
// 逐步核验，数据库双方行锁负责决定哪一个候选能成为新的已提交日志。
type Authority struct {
	mu           sync.Mutex
	config       Config
	resourceHash string
	process      *Process
	head         string
	reply        json.RawMessage
}

func NewAuthority(config Config, resourceHash string) (*Authority, error) {
	if !validDigest(resourceHash) {
		return nil, errors.New("原生权威必须绑定已核验资源SHA256")
	}
	return &Authority{config: config, resourceHash: resourceHash}, nil
}

func validDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && value == hex.EncodeToString(raw)
}

// 保留JSON整数精度，统一字典键顺序，拒绝第二个JSON值。
func canonicalJSON(raw []byte) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, errors.New("原生日志包含第二个JSON值")
	} else if err != io.EOF {
		return nil, err
	}
	return json.Marshal(value)
}

func hashJSON(raw []byte) (string, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func chainedHash(previous string, request json.RawMessage, replyDigest string) string {
	// PostgreSQL jsonb会重排对象键；哈希链必须绑定JSON语义而不是存储层返回的字节顺序。
	canonical, err := canonicalJSON(request)
	if err != nil {
		return ""
	}
	body, _ := json.Marshal([]any{JournalVersion, previous, canonical, replyDigest})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func decodeRequest(raw json.RawMessage, start bool) (map[string]any, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	var request map[string]any
	if err = decoder.Decode(&request); err != nil {
		return nil, err
	}
	operation, _ := request["operation"].(string)
	if start {
		if operation != "start" {
			return nil, errors.New("原生日志首项必须是冻结阵容启动")
		}
	} else {
		switch operation {
		case "drive", "timeout":
			if len(request) != 1 {
				return nil, errors.New("原生推进请求含未定义字段")
			}
		case "step":
			if len(request) != 2 || request["command"] == nil {
				return nil, errors.New("原生输入请求结构无效")
			}
		case "set_auto":
			_, idOK := request["avatar_id"].(string)
			_, boolOK := request["enabled"].(bool)
			if len(request) != 3 || !idOK || !boolOK {
				return nil, errors.New("原生自动状态请求结构无效")
			}
		default:
			return nil, errors.New("原生共同日志不接受该操作")
		}
	}
	return request, nil
}

func (a *Authority) validate(j *Journal) error {
	if j == nil || j.Version != JournalVersion || j.ResourceHash != a.resourceHash || !validDigest(j.StartDigest) || len(j.Entries) > MaxJournalEntries {
		return errors.New("原生日志版本、资源或长度不符")
	}
	if _, err := decodeRequest(j.Start, true); err != nil {
		return err
	}
	head := chainedHash(j.ResourceHash, j.Start, j.StartDigest)
	for _, entry := range j.Entries {
		if _, err := decodeRequest(entry.Request, false); err != nil {
			return err
		}
		if !validDigest(entry.ReplyDigest) {
			return errors.New("原生日志回复校验值无效")
		}
		head = chainedHash(head, entry.Request, entry.ReplyDigest)
		if entry.Head != head {
			return errors.New("原生日志输入链被覆盖或截断")
		}
	}
	if j.Head != head {
		return errors.New("原生日志末端校验不符")
	}
	encoded, err := json.Marshal(j)
	if err != nil || len(encoded) > MaxJournalBytes {
		return errors.New("原生日志超过保护上限")
	}
	return nil
}

func (a *Authority) discard() {
	if a.process != nil {
		a.process.Close()
	}
	a.process, a.head, a.reply = nil, "", nil
}

func (a *Authority) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.discard()
}

// Begin的返回值同样只是候选。数据库失败后丢弃返回值即可，随后Restore会
// 用数据库中原日志重新启动，撤销未提交的引擎推进。
func (a *Authority) Begin(ctx context.Context, request map[string]any) (*Journal, json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	raw, err = canonicalJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := decodeRequest(raw, true)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > MaxJournalBytes/2 {
		return nil, nil, errors.New("原生冻结阵容超过保护上限")
	}
	a.discard()
	a.process, err = Open(ctx, a.config)
	if err != nil {
		return nil, nil, err
	}
	reply, err := a.process.Request(ctx, parsed)
	if err != nil {
		a.discard()
		return nil, nil, err
	}
	digest, err := hashJSON(reply)
	if err != nil {
		a.discard()
		return nil, nil, err
	}
	j := &Journal{Version: JournalVersion, ResourceHash: a.resourceHash, Start: raw, StartDigest: digest}
	j.Head = chainedHash(j.ResourceHash, j.Start, j.StartDigest)
	a.head, a.reply = j.Head, append(json.RawMessage(nil), reply...)
	return j, append(json.RawMessage(nil), reply...), nil
}

func (a *Authority) restore(ctx context.Context, j *Journal) error {
	if err := a.validate(j); err != nil {
		return err
	}
	if a.process != nil && a.head == j.Head {
		return nil
	}
	a.discard()
	var err error
	a.process, err = Open(ctx, a.config)
	if err != nil {
		return err
	}
	requests := append([]JournalEntry{{Request: j.Start, ReplyDigest: j.StartDigest}}, j.Entries...)
	for i, entry := range requests {
		request, err := decodeRequest(entry.Request, i == 0)
		if err != nil {
			a.discard()
			return err
		}
		reply, err := a.process.Request(ctx, request)
		if err != nil {
			a.discard()
			return fmt.Errorf("原生已提交日志第%d项重放失败：%w", i, err)
		}
		digest, err := hashJSON(reply)
		if err != nil || digest != entry.ReplyDigest {
			a.discard()
			return fmt.Errorf("原生已提交日志第%d项重放结果分歧，禁止继续计分", i)
		}
		a.reply = append(json.RawMessage(nil), reply...)
	}
	a.head = j.Head
	return nil
}

func (a *Authority) Restore(ctx context.Context, j *Journal) (json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.restore(ctx, j); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), a.reply...), nil
}

// Advance不会改写输入日志。成功时进程停在候选末态；只有调用方提交返回日志，
// 下次请求才可复用。原生日志和房间成绩不能分开提交。
func (a *Authority) Advance(ctx context.Context, committed *Journal, request map[string]any) (*Journal, json.RawMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.restore(ctx, committed); err != nil {
		return nil, nil, err
	}
	if len(committed.Entries) >= MaxJournalEntries {
		return nil, nil, errors.New("原生输入日志达到保护上限")
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}
	raw, err = canonicalJSON(raw)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := decodeRequest(raw, false)
	if err != nil {
		return nil, nil, err
	}
	// 先核验候选尺寸，不能执行后才发现持久日志无法保存。
	clone := *committed
	clone.Start = append(json.RawMessage(nil), committed.Start...)
	clone.Entries = make([]JournalEntry, len(committed.Entries)+1)
	for i, old := range committed.Entries {
		clone.Entries[i] = old
		clone.Entries[i].Request = append(json.RawMessage(nil), old.Request...)
	}
	clone.Entries[len(committed.Entries)] = JournalEntry{Request: raw, ReplyDigest: committed.StartDigest, Head: committed.Head}
	encoded, _ := json.Marshal(&clone)
	if len(encoded) > MaxJournalBytes {
		return nil, nil, errors.New("原生输入日志超过保护上限")
	}
	reply, err := a.process.Request(ctx, parsed)
	if err != nil {
		var rejected *Rejection
		if !errors.As(err, &rejected) {
			a.discard()
		}
		return nil, nil, err
	}
	digest, err := hashJSON(reply)
	if err != nil {
		a.discard()
		return nil, nil, err
	}
	clone.Head = chainedHash(committed.Head, raw, digest)
	clone.Entries[len(committed.Entries)] = JournalEntry{Request: raw, ReplyDigest: digest, Head: clone.Head}
	a.head, a.reply = clone.Head, append(json.RawMessage(nil), reply...)
	return &clone, append(json.RawMessage(nil), reply...), nil
}
