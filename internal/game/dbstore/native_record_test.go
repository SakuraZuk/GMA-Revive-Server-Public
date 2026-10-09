package dbstore

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hs-server/internal/game"
	"strconv"
	"sync"
	"testing"
	"time"
)

type pgNativeTuple []any

func pgNativeFile(owner, enemy string) []byte {
	buf := &bytes.Buffer{}
	var emit func(any)
	emit = func(x any) {
		switch v := x.(type) {
		case string:
			buf.WriteString("S" + strconv.Quote(v) + "\n")
		case int:
			buf.WriteString(fmt.Sprintf("I%d\n", v))
		case []any:
			buf.WriteString("(l")
			for _, e := range v {
				emit(e)
				buf.WriteByte('a')
			}
		case pgNativeTuple:
			buf.WriteByte('(')
			for _, e := range v {
				emit(e)
			}
			buf.WriteByte('t')
		case map[string]any:
			buf.WriteString("(d")
			for k, e := range v {
				emit(k)
				emit(e)
				buf.WriteByte('s')
			}
		}
	}
	rows := []any{pgNativeTuple{0, "set_last_fighting_cards", []any{map[string]any{owner: []any{"卡"}}}, map[string]any{}}, pgNativeTuple{1, "prepare", []any{[]any{owner, enemy}, 1, 42, 1}, map[string]any{}}, pgNativeTuple{2, "add_fighting_cards", []any{"原生夹具"}, map[string]any{}}, pgNativeTuple{3, "battle_end_notice", []any{[]any{owner}, 0}, map[string]any{}}}
	emit(rows)
	buf.WriteByte('.')
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	_, _ = z.Write(buf.Bytes())
	_ = z.Close()
	return compressed.Bytes()
}

func TestPostgresNativeRecordActualUploadPlaybackOwnershipAndRollback(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	identities := []game.Identity{}
	for _, name := range []string{"原生录像攻击者", "原生录像防守者", "原生录像第三人"} {
		identity, err := store.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		identities = append(identities, identity)
	}
	owner := hex.EncodeToString(identities[0].Avatars[0].OID)
	enemy := hex.EncodeToString(identities[1].Avatars[0].OID)
	uuid := "123456789012345678901234"
	if _, err := store.UpdateProgress(ctx, identities[0].Avatars[0].OID, func(p *game.Progress) error {
		p.AsyncPvp.AttackRecords = []game.AsyncPvpRecord{{UUID: uuid, EnemyInfo: game.SocialProfile{EID: enemy}, Win: true, Time: now.Unix()}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateProgress(ctx, identities[1].Avatars[0].OID, func(p *game.Progress) error {
		p.AsyncPvp.DefenceRecords = []game.AsyncPvpRecord{{UUID: uuid, EnemyInfo: game.SocialProfile{EID: owner}, Win: false, Time: now.Unix()}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	services := []*game.Service{}
	cs := []*game.Connection{}
	for _, index := range []int{0, 0, 1, 2} {
		svc := pgSocialService(t, New(store.pool))
		svc.Now = func() time.Time { return now }
		c := pgSocialLogin(t, svc, game.ClientInfo{Account: identities[index].Account, Password: "pw", Hostnum: 1}, identities[index].Avatars[0].OID)
		services = append(services, svc)
		cs = append(cs, c)
		defer svc.Detach(c)
	}
	data := pgNativeFile(owner, enemy)
	hash := sha256.Sum256(data)
	chunk := game.NativeRecordChunk{UUID: uuid, Version: "1.0.128", SHA256: hex.EncodeToString(hash[:]), Size: len(data), Count: 4, Index: 0, Total: 1, Data: base64.StdEncoding.EncodeToString(data)}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pushes, err := services[i].Handle(ctx, cs[i], "upload_native_battle_record", pgSocialArgs(1, chunk))
			if err == nil && (len(pushes) != 1 || pushes[0].Args[1].([]any)[0] != true) {
				err = errors.New("原生录像同哈希并发上传失败")
			}
			if err != nil {
				errs <- err
			}
		}(n % 2)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	file, err := New(store.pool).ReadNativeRecord(ctx, identities[1].Avatars[0].OID, uuid)
	if err != nil || !bytes.Equal(file.Data, data) {
		t.Fatal("完整文件/防守者共享/重建Store持久失败", err)
	}
	stranger, err := store.ReadNativeRecord(ctx, identities[2].Avatars[0].OID, uuid)
	if err != nil || len(stranger.Data) != 0 {
		t.Fatal("陌生角色读取录像", err)
	}
	_, err = store.UpdateNativeRecord(ctx, identities[0].Avatars[0].OID, uuid, func(av *game.Avatar, r *game.NativeBattleRecord) error {
		r.Data = []byte("故障")
		return errors.New("录像整文件故障注入")
	})
	if err == nil {
		t.Fatal("故障未回滚")
	}
	file, err = store.ReadNativeRecord(ctx, identities[0].Avatars[0].OID, uuid)
	if err != nil || !bytes.Equal(file.Data, data) {
		t.Fatal("录像故障部分提交", err)
	}
	for i, kind := range map[int]int{0: 16, 2: 4} {
		resultType := 8
		if kind == 4 {
			resultType = 2
		}
		if _, err := services[i].Handle(ctx, cs[i], "get_record_result", pgSocialArgs(resultType)); err != nil {
			t.Fatal(err)
		}
		pushes, err := services[i].Handle(ctx, cs[i], "start_record_battle", pgSocialArgs(0, "1.0.128", kind))
		if err != nil || len(pushes) != 1 || pushes[0].Method != "real_start_record_battle" || !bytes.Equal(pushes[0].Args[0].([]byte), data) {
			t.Fatal("原生4/16完整播放文件未对应本人列表", err)
		}
	}
	chunk.Version = "1.0.125"
	pushes, err := services[0].Handle(ctx, cs[0], "upload_native_battle_record", pgSocialArgs(2, chunk))
	if err != nil || pushes[0].Args[1].([]any)[0] != false {
		t.Fatal("版本冲突被接受", err)
	}
}
