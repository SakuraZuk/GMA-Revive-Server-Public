package game

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func nativeTestWrite(v any) *bytes.Buffer {
	b := &bytes.Buffer{}
	var emit func(any)
	emit = func(x any) {
		switch v := x.(type) {
		case nil:
			b.WriteString("N")
		case string:
			b.WriteString("S" + strconv.Quote(v) + "\n")
		case int:
			b.WriteString(fmt.Sprintf("I%d\n", v))
		case float64:
			b.WriteString(fmt.Sprintf("F%g\n", v))
		case []any:
			b.WriteString("(l")
			for _, item := range v {
				emit(item)
				b.WriteByte('a')
			}
		case map[string]any:
			b.WriteString("(d")
			keys := []string{}
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				emit(k)
				emit(v[k])
				b.WriteByte('s')
			}
		case nativeTestTuple:
			b.WriteByte('(')
			for _, item := range v {
				emit(item)
			}
			b.WriteByte('t')
		default:
			panic(fmt.Sprintf("测试pickle类型%T", x))
		}
	}
	emit(v)
	b.WriteByte('.')
	return b
}

type nativeTestTuple []any

func nativeTestRecording(owner, enemy string, payload string) []byte {
	rows := []any{nativeTestTuple{0.0, "set_last_fighting_cards", []any{map[string]any{owner: []any{"卡"}}}, map[string]any{}}, nativeTestTuple{1.0, "prepare", []any{[]any{owner, enemy}, 1, 123, asyncRule().DungeonID}, map[string]any{}}, nativeTestTuple{2.0, "add_fighting_cards", []any{payload}, map[string]any{}}, nativeTestTuple{3.0, "battle_end_notice", []any{[]any{owner}, 0}, map[string]any{}}}
	return nativeTestWrite(rows).Bytes()
}
func TestNativeRecordStaticPickleRealSamplesAndExecutionRejection(t *testing.T) {
	for filename, want := range map[string]string{"../../files/netease/h62/Documents/battle.txt": "ae1e95e79c0be72bb3972f16ebb9ec2ac8c3a94cb3c74ed0a5a4ef1782c70364", "../../com.netease.hsqsl/files/netease/h62/Documents/battle.txt": "55addc5ebe2a6ba1d31076f4b40fb11e25b22a287610963cb75c1c62be6a70dd"} {
		t.Run(want[:8], func(t *testing.T) {
			raw, err := os.ReadFile(filename)
			if os.IsNotExist(err) {
				t.Skip("真实客户端录像未配置；公开源码不携带玩家数据，本样本未验收")
			}
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(raw)
			if hex.EncodeToString(hash[:]) != want {
				t.Fatal("本版原生样本SHA变化")
			}
			root, err := parseNativePickle(raw)
			if err != nil || root.kind != 'l' || len(root.items) == 0 {
				t.Fatal("原生protocol0/ObjectID无法静态恢复", filename, err)
			}
			if err := ValidateNativeRecordPickle(raw, len(root.items)); err != nil {
				t.Fatal("本版原生完整文件不兼容", err)
			}
		})
	}
	for _, raw := range []string{"cos\nsystem\n(S'command'\ntR.", "(lp0\ng0\na.", "(dS'__custom_type'\nS'__import__(\"os\").system'\ns.", "(cbson.objectid\nObjectId\nc__builtin__\nobject\nNtcother\ncall\nR."} {
		if _, err := parseNativePickle([]byte(raw)); err == nil {
			t.Fatal("执行或循环数据被接受", raw)
		}
	}
	owner, enemy := "123456789012345678901234", "234567890123456789012345"
	raw := nativeTestRecording(owner, enemy, "完整原生夹具")
	if err := ValidateNativeRecordPickle(raw, 4); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeRecordOwnership(raw, owner, AsyncPvpRecord{EnemyInfo: SocialProfile{EID: enemy}, Win: true}); err != nil {
		t.Fatal(err)
	}
	if validateNativeRecordOwnership(raw, owner, AsyncPvpRecord{EnemyInfo: SocialProfile{EID: enemy}, Win: false}) == nil {
		t.Fatal("录像胜方可覆盖结果收据")
	}
}

func TestNativeRecordChunksAtomicHashVersionOwnershipAndOriginalPlayback(t *testing.T) {
	s, cs, store := socialTestWorld(t)
	c := cs[0]
	owner := hexOf(selectedOID(c))
	enemy := hexOf(selectedOID(cs[1]))
	uuid := "123456789012345678901234"
	_, err := store.UpdateProgress(context.Background(), selectedOID(c), func(p *Progress) error {
		p.AsyncPvp.AttackRecords = []AsyncPvpRecord{{UUID: uuid, EnemyInfo: SocialProfile{EID: enemy}, Win: true, Time: s.Now().Unix()}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	text := &strings.Builder{}
	for i := 0; i < 5000; i++ {
		h := sha256.Sum256([]byte(fmt.Sprint(i)))
		text.WriteString(hex.EncodeToString(h[:]))
	}
	raw := nativeTestRecording(owner, enemy, text.String())
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	_, _ = z.Write(raw)
	_ = z.Close()
	data := compressed.Bytes()
	hash := sha256.Sum256(data)
	total := (len(data) + nativeRecordChunkSize - 1) / nativeRecordChunkSize
	if total < 2 {
		t.Fatal("分块夹具未跨边界")
	}
	chunks := []NativeRecordChunk{}
	for i := 0; i < total; i++ {
		end := (i + 1) * nativeRecordChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, NativeRecordChunk{UUID: uuid, Version: "1.0.128", SHA256: hex.EncodeToString(hash[:]), Size: len(data), Count: 4, Index: i, Total: total, Data: base64.StdEncoding.EncodeToString(data[i*nativeRecordChunkSize : end])})
	}
	if _, err := store.UpdateNativeRecord(context.Background(), selectedOID(c), uuid, func(av *Avatar, r *NativeBattleRecord) error {
		return acceptNativeRecordChunk(av, r, chunks[0], s.Now().Unix())
	}); err != nil {
		t.Fatal(err)
	}
	partial, _ := store.ReadNativeRecord(context.Background(), selectedOID(c), uuid)
	if len(partial.Data) != 0 || len(partial.Chunks) != 1 {
		t.Fatal("部分文件被开放播放")
	}
	conflict := chunks[0]
	conflict.Version = "1.0.125"
	if _, err := store.UpdateNativeRecord(context.Background(), selectedOID(c), uuid, func(av *Avatar, r *NativeBattleRecord) error {
		return acceptNativeRecordChunk(av, r, conflict, s.Now().Unix())
	}); err == nil {
		t.Fatal("错误版本被接受")
	}
	for _, chunk := range chunks {
		for n := 0; n < 2; n++ {
			if _, err := store.UpdateNativeRecord(context.Background(), selectedOID(c), uuid, func(av *Avatar, r *NativeBattleRecord) error {
				return acceptNativeRecordChunk(av, r, chunk, s.Now().Unix())
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	complete, _ := store.ReadNativeRecord(context.Background(), selectedOID(cs[1]), uuid)
	if !bytes.Equal(complete.Data, data) || len(complete.Chunks) != 0 {
		t.Fatal("完整文件未保持原始字节或防守者不可读")
	}
	stranger, _ := store.ReadNativeRecord(context.Background(), selectedOID(cs[2]), uuid)
	if len(stranger.Data) != 0 {
		t.Fatal("第三角色可以读他人录像")
	}
	if err := s.refreshHumanConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.getRecordResult(context.Background(), c, socialArgs(8)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProgress(context.Background(), selectedOID(c), func(p *Progress) error {
		p.AsyncPvp.AttackRecords = append(p.AsyncPvp.AttackRecords, AsyncPvpRecord{UUID: "345678901234567890123456", Time: s.Now().Unix() + 1})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.refreshHumanConnection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	pushes, err := s.startNativeRecordBattle(context.Background(), c, socialArgs(0, "1.0.128", 16))
	if err != nil || len(pushes) != 1 || pushes[0].Method != "real_start_record_battle" || !bytes.Equal(pushes[0].Args[0].([]byte), data) {
		t.Fatal("原生播放未返回同一完整zlib文件", err)
	}
}
