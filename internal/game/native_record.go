package game

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

//go:embed native_record_bridge_script.py
var nativeRecordBridgeScript string

const nativeRecordCompressedMax = 768 * 1024
const nativeRecordExpandedMax = 64 * 1024 * 1024
const nativeRecordChunkSize = 49152

type NativeBattleRecord struct {
	UUID    string         `json:"uuid"`
	Version string         `json:"version"`
	SHA256  string         `json:"sha256"`
	Size    int            `json:"size"`
	Count   int            `json:"count"`
	Total   int            `json:"total"`
	Chunks  map[int]string `json:"chunks,omitempty"`
	Data    []byte         `json:"data,omitempty"`
	Owner   string         `json:"owner"`
	Viewers []string       `json:"viewers"`
	Created int64          `json:"created"`
}
type NativeRecordChunk struct {
	UUID    string `json:"battle_uuid"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Size    int    `json:"size"`
	Count   int    `json:"count"`
	Index   int    `json:"index"`
	Total   int    `json:"total"`
	Data    string `json:"data"`
}
type NativeBattleRecordAccounts interface {
	UpdateNativeRecord(context.Context, []byte, string, func(*Avatar, *NativeBattleRecord) error) (NativeBattleRecord, error)
	ReadNativeRecord(context.Context, []byte, string) (NativeBattleRecord, error)
}

func cloneNativeRecord(v NativeBattleRecord) NativeBattleRecord {
	raw, _ := json.Marshal(v)
	var out NativeBattleRecord
	_ = json.Unmarshal(raw, &out)
	return out
}

var nativeFixtureRecords sync.Map

func (a *FixtureAccounts) UpdateNativeRecord(ctx context.Context, oid []byte, uuid string, fn func(*Avatar, *NativeBattleRecord) error) (NativeBattleRecord, error) {
	if err := ctx.Err(); err != nil {
		return NativeBattleRecord{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	rows := fixtureSocialAvatars(a.records, SocialSearch{OIDs: []string{hexOf(oid)}, Limit: 1})
	if len(rows) != 1 {
		return NativeBattleRecord{}, errors.New("录像上传角色不存在")
	}
	value, _ := nativeFixtureRecords.LoadOrStore(a, map[string]NativeBattleRecord{})
	records := value.(map[string]NativeBattleRecord)
	record := cloneNativeRecord(records[uuid])
	if err := fn(&rows[0], &record); err != nil {
		return NativeBattleRecord{}, err
	}
	records[uuid] = cloneNativeRecord(record)
	return record, nil
}
func (a *FixtureAccounts) ReadNativeRecord(ctx context.Context, oid []byte, uuid string) (NativeBattleRecord, error) {
	if err := ctx.Err(); err != nil {
		return NativeBattleRecord{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	value, ok := nativeFixtureRecords.Load(a)
	if !ok {
		return NativeBattleRecord{}, nil
	}
	record := cloneNativeRecord(value.(map[string]NativeBattleRecord)[uuid])
	for _, id := range record.Viewers {
		if id == hexOf(oid) {
			return record, nil
		}
	}
	return NativeBattleRecord{}, nil
}

func nativeUploadReceipt(av *Avatar, uuid string) (AsyncPvpRecord, error) {
	for _, record := range av.Progress.AsyncPvp.AttackRecords {
		if record.UUID == uuid {
			return record, nil
		}
	}
	return AsyncPvpRecord{}, errors.New("录像必须属于当前鉴权角色真实已结算的异步攻击记录")
}
func acceptNativeRecordChunk(av *Avatar, record *NativeBattleRecord, chunk NativeRecordChunk, now int64) error {
	receipt, err := nativeUploadReceipt(av, chunk.UUID)
	if err != nil {
		return err
	}
	owner := hexOf(av.OID)
	if chunk.Version != "1.0.128" || !validObjectID(chunk.UUID) || len(chunk.SHA256) != 64 || chunk.Count < 4 || chunk.Count > 100000 || chunk.Size <= 0 || chunk.Size > nativeRecordCompressedMax || chunk.Total != (chunk.Size+nativeRecordChunkSize-1)/nativeRecordChunkSize || chunk.Index < 0 || chunk.Index >= chunk.Total {
		return errors.New("原生录像版本、大小或分块参数无效")
	}
	if _, err := hex.DecodeString(chunk.SHA256); err != nil {
		return errors.New("原生录像SHA256无效")
	}
	part, err := base64.StdEncoding.Strict().DecodeString(chunk.Data)
	if err != nil {
		return errors.New("原生录像分块base64无效")
	}
	expect := nativeRecordChunkSize
	if chunk.Index == chunk.Total-1 {
		expect = chunk.Size - chunk.Index*nativeRecordChunkSize
	}
	if len(part) != expect {
		return errors.New("原生录像分块长度不符")
	}
	if record.UUID == "" {
		*record = NativeBattleRecord{UUID: chunk.UUID, Version: chunk.Version, SHA256: chunk.SHA256, Size: chunk.Size, Count: chunk.Count, Total: chunk.Total, Chunks: map[int]string{}, Owner: owner, Viewers: []string{owner}, Created: now}
		if receipt.EnemyInfo.EID != "" {
			record.Viewers = append(record.Viewers, receipt.EnemyInfo.EID)
		}
	}
	if record.UUID != chunk.UUID || record.Owner != owner || record.Version != chunk.Version || record.SHA256 != chunk.SHA256 || record.Size != chunk.Size || record.Count != chunk.Count || record.Total != chunk.Total {
		return errors.New("原生录像已冻结元数据，不允许覆盖")
	}
	if len(record.Data) > 0 {
		if !bytes.Equal(record.Data[chunk.Index*nativeRecordChunkSize:chunk.Index*nativeRecordChunkSize+len(part)], part) {
			return errors.New("原生录像完成后的重试内容冲突")
		}
		return nil
	}
	if old, ok := record.Chunks[chunk.Index]; ok && old != chunk.Data {
		return errors.New("原生录像分块重试内容冲突")
	}
	record.Chunks[chunk.Index] = chunk.Data
	if len(record.Chunks) < record.Total {
		return nil
	}
	data := make([]byte, 0, record.Size)
	for i := 0; i < record.Total; i++ {
		part, e := base64.StdEncoding.Strict().DecodeString(record.Chunks[i])
		if e != nil {
			return e
		}
		data = append(data, part...)
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != record.SHA256 {
		return errors.New("原生录像完整SHA256不符")
	}
	z, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return errors.New("原生录像zlib校验失败")
	}
	raw, err := io.ReadAll(io.LimitReader(z, nativeRecordExpandedMax+1))
	closeErr := z.Close()
	if err != nil || closeErr != nil || len(raw) > nativeRecordExpandedMax {
		return errors.New("原生录像解压失败或超出完整文件保护上限")
	}
	if err := ValidateNativeRecordPickle(raw, record.Count); err != nil {
		return err
	}
	if err := validateNativeRecordOwnership(raw, owner, receipt); err != nil {
		return err
	}
	record.Data = data
	record.Chunks = nil
	return nil
}

func (s *Service) uploadNativeBattleRecord(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	cb, ok := callbackArg(args)
	if c.phase != Playing || !ok || len(args) != 2 {
		return nil, errors.New("原生录像上传需要鉴权角色、回调及分块")
	}
	var chunk NativeRecordChunk
	if json.Unmarshal(args[1], &chunk) != nil {
		return nil, errors.New("原生录像分块参数无效")
	}
	store, ok := s.Accounts.(NativeBattleRecordAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持原生录像")
	}
	_, err := store.UpdateNativeRecord(ctx, selectedOID(c), chunk.UUID, func(av *Avatar, record *NativeBattleRecord) error {
		return acceptNativeRecordChunk(av, record, chunk, s.Now().Unix())
	})
	if err != nil {
		return []Push{Callback(cb, []any{false, err.Error()})}, nil
	}
	return []Push{Callback(cb, []any{true, ""})}, nil
}

func (s *Service) startNativeRecordBattle(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 3 {
		return nil, errors.New("原生录像播放需要索引、版本及4/16类型")
	}
	var index, kind int
	var version string
	if json.Unmarshal(args[0], &index) != nil || json.Unmarshal(args[1], &version) != nil || json.Unmarshal(args[2], &kind) != nil || index < 0 || index >= 10 || version != "1.0.128" || (kind != 4 && kind != 16) {
		return []Push{push("Avatar", "real_start_record_battle", []byte{})}, nil
	}
	store, ok := s.Accounts.(NativeBattleRecordAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持原生录像")
	}
	if b := c.SelectedAvatarUnsafe().Progress.Battle; b != nil && !b.Finished {
		return nil, errors.New("正在战斗时不能进入录像")
	}
	// 与原生结果界面完全相同的排序和当前角色列表；索引不能改为上传的UUID绕过归属。
	list := c.nativeRecordLists[kind]
	if index >= len(list) {
		return []Push{push("Avatar", "real_start_record_battle", []byte{})}, nil
	}
	record, err := store.ReadNativeRecord(ctx, selectedOID(c), list[index])
	if err != nil {
		return nil, err
	}
	if record.Version != version || len(record.Data) == 0 || record.UUID != list[index] {
		return []Push{push("Avatar", "real_start_record_battle", []byte{})}, nil
	}
	return []Push{push("Avatar", "real_start_record_battle", record.Data)}, nil
}
