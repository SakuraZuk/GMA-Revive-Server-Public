package nativeengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

const testResourceHash = "bed95c22e6e6adf4ccf01cabb5156b4888ccc5744f0e192d24501e6df3c19ef3"

func journalClone(t *testing.T, j *Journal) *Journal {
	t.Helper()
	raw, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	var out Journal
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return &out
}

func journalDuelStart(t *testing.T, ctx context.Context, config Config) map[string]any {
	t.Helper()
	p, err := Open(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	roles, err := p.Request(ctx, map[string]any{"operation": "table", "table": "role_info", "keys": []int{1601, 2103, 2202, 4402}, "fields": []string{"skill_list"}})
	if err != nil {
		t.Fatal(err)
	}
	var table map[int]struct {
		Skills []int `json:"skill_list"`
	}
	if err = json.Unmarshal(roles, &table); err != nil {
		t.Fatal(err)
	}
	roster := func(side int) []any {
		var cards []any
		for i, id := range []int{1601, 2103, 2202, 4402} {
			var skills []any
			for _, skill := range table[id].Skills {
				skills = append(skills, map[string]any{"skill_id": skill, "level": 1, "enhance_level": 0})
			}
			cards = append(cards, map[string]any{"card_id": id, "uuid": fmt.Sprintf("%024x", side*100+i+1), "level": 40, "grade": 4, "awakened": 1, "skill_mgr": skills})
		}
		return cards
	}
	return map[string]any{"operation": "start", "metadata": map[string]any{"dungeon_id": 21, "avatar_id": "000000000000000000000001", "enemy_id": "000000000000000000000002", "battle_uuid": "000000000000000000000003", "enemy_roster": roster(2), "battle_type": 2, "enemy_auto": false}, "roster": roster(1), "seed": 123456, "auto": false}
}

func TestNativeJournalActualRollbackRetryAndColdReplay(t *testing.T) {
	config := nativeTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := NewAuthority(config, testResourceHash)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	j, _, err := a.Begin(ctx, journalDuelStart(t, ctx, config))
	if err != nil {
		t.Fatal(err)
	}
	j, waiting, err := a.Advance(ctx, j, map[string]any{"operation": "drive"})
	if err != nil {
		t.Fatal(err)
	}
	// 模拟JSONB冷读取；输入的已提交日志必须保持完整不变。
	committed := journalClone(t, j)
	before, _ := json.Marshal(committed)
	_, _, err = a.Advance(ctx, committed, map[string]any{"operation": "step", "command": map[string]any{"name": "move_to", "args": []any{"不存在的单位", []int{0, 0, 0}}}})
	var rejected *Rejection
	if !errors.As(err, &rejected) {
		t.Fatal("非法输入没有保留原生拒绝类别", err)
	}
	restored, err := a.Restore(ctx, committed)
	if err != nil || !bytes.Equal(waiting, restored) {
		t.Fatal("拒绝后的已提交原生回复失配", err)
	}
	candidate, timeoutReply, err := a.Advance(ctx, committed, map[string]any{"operation": "timeout"})
	if err != nil {
		t.Fatal(err)
	}
	// 丢弃candidate模拟SQL提交失败。引擎已经执行，但重试仍从committed重建。
	restored, err = a.Restore(ctx, committed)
	if err != nil || !bytes.Equal(waiting, restored) {
		t.Fatal("未提交推进没有回到原生已提交窗口", err)
	}
	retry, retryReply, err := a.Advance(ctx, committed, map[string]any{"operation": "timeout"})
	if err != nil || retry.Head != candidate.Head || !bytes.Equal(timeoutReply, retryReply) {
		t.Fatal("SQL失败后的同一输入重试重复推进或结果分歧", err)
	}
	after, _ := json.Marshal(committed)
	if !bytes.Equal(before, after) {
		t.Fatal("候选推进篡改了调用方已提交日志")
	}
	// 完整丢弃进程，使用另一份权威与反序列化日志冷重建。
	a.Close()
	b, err := NewAuthority(config, testResourceHash)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	restored, err = b.Restore(ctx, journalClone(t, retry))
	if err != nil || !bytes.Equal(retryReply, restored) {
		t.Fatal("跨进程冷重建与原始已提交结果不同", err)
	}
	// 即使重算输入链，伪造的旧回复摘要也必须被真实重放识别。
	bad := journalClone(t, retry)
	bad.Entries[0].ReplyDigest = "0000000000000000000000000000000000000000000000000000000000000000"
	head := chainedHash(bad.ResourceHash, bad.Start, bad.StartDigest)
	for i := range bad.Entries {
		head = chainedHash(head, bad.Entries[i].Request, bad.Entries[i].ReplyDigest)
		bad.Entries[i].Head = head
	}
	bad.Head = head
	if _, err = b.Restore(ctx, bad); err == nil {
		t.Fatal("篡改回复摘要后原生重放分歧仍被接受")
	}
	// 错误日志不能污染正确房间，仍能从有效日志恢复。
	restored, err = b.Restore(ctx, retry)
	if err != nil || !bytes.Equal(retryReply, restored) {
		t.Fatal("重放拒绝后正确日志不能重新恢复", err)
	}
}

func TestNativeJournalRejectsInvalidOperationsAndPrecisionLoss(t *testing.T) {
	if _, err := NewAuthority(Config{}, "不合法"); err == nil {
		t.Fatal("缺失资源绑定被接受")
	}
	for _, raw := range []string{`{} {}`, `{"a":1} garbage`, `{"operation":"snapshot"}`, `{"operation":"drive","metadata":{}}`, `{"operation":"set_auto","avatar_id":"x","enabled":1}`} {
		if _, err := decodeRequest(json.RawMessage(raw), false); err == nil {
			t.Fatal("日志接受了非法请求", raw)
		}
	}
	canonical, err := canonicalJSON([]byte(`{"z":9223372036854775807,"a":1}`))
	if err != nil || !bytes.Equal(canonical, []byte(`{"a":1,"z":9223372036854775807}`)) {
		t.Fatal("共同日志整数被float64截断", string(canonical), err)
	}
	left := chainedHash(testResourceHash, json.RawMessage(`{"operation":"step","command":{"name":"move_to","args":["1",[1,-1,0]]}}`), testResourceHash)
	right := chainedHash(testResourceHash, json.RawMessage(`{"command":{"args":["1",[1,-1,0]],"name":"move_to"},"operation":"step"}`), testResourceHash)
	if left == "" || left != right {
		t.Fatal("JSONB重排对象键后改变了原生日志哈希链")
	}
}
