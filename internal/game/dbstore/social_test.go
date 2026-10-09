package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hs-server/internal/game"
	"hs-server/internal/hotfix"
	"hs-server/internal/mobileproto"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// PG业务链仍走原生quick_login单client_info实参；加载完整清单，禁止nil热更夹具。
func pgSocialService(t *testing.T, accounts game.Accounts) *game.Service {
	t.Helper()
	catalog := &hotfix.Catalog{}
	if err := catalog.Load(filepath.Join("..", "..", "..", "deploy", "data", "hotfix.json")); err != nil {
		t.Fatal("PG夹具无法加载完整热更清单", err)
	}
	return game.New(accounts, catalog)
}

func pgSocialLogin(t *testing.T, svc *game.Service, info game.ClientInfo, wantOID []byte) *game.Connection {
	t.Helper()
	if svc.Hotfix == nil {
		t.Fatal("PG登录夹具禁止nil热更目录")
	}
	c := game.NewConnection()
	pushes, err := svc.Handle(context.Background(), c, "quick_login", pgSocialArgs(info))
	if err != nil {
		t.Fatal("PG账号登录失败", err)
	}
	loginOK, hotfixOK := false, false
	expected := svc.Hotfix.Query(info.HotfixIndex)
	for _, item := range pushes {
		switch item.Method {
		case "login_result":
			loginOK = len(item.Args) == 3 && item.Args[0] == 0 && item.Args[2] == info.ConnType
		case "on_hotfix_when_login":
			hotfixOK = len(item.Args) == 2 && item.Args[0] == expected.Script && item.Args[1] == expected.Index && expected.Script != ""
		}
	}
	if !loginOK || !hotfixOK || c.Phase() != game.Authenticated {
		t.Fatal("PG账号登录未得到成功及完整热更推送", pushes)
	}
	selected, ok := c.SelectedAvatar()
	if !ok || selected.Hostnum != info.Hostnum || !bytes.Equal(selected.OID, wantOID) {
		t.Fatal("PG业务登录未选中当前真实角色", selected, info.Hostnum)
	}
	if _, err = svc.BecomePlayer(c); err != nil || c.Phase() != game.Playing {
		t.Fatal("PG登录角色未进入Playing", err)
	}
	t.Cleanup(func() { svc.Detach(c) })
	return c
}

// 本机协议夹具检查，不连接PG；数据库用例仍由testStore显式跳过或真实运行。
func TestSocialLoginFixtureProtocolAndCompleteHotfix(t *testing.T) {
	accounts := game.NewFixtureAccounts(nil)
	info := game.ClientInfo{Account: "PG夹具协议检查", Password: "pw", Hostnum: 1, ConnType: 1}
	identity, err := accounts.Register(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	svc := pgSocialService(t, accounts)
	c := pgSocialLogin(t, svc, info, identity.Avatars[0].OID)
	if c.Phase() != game.Playing {
		t.Fatal("原生业务协议夹具未进入Playing")
	}
}

func TestPostgresSocialPairsLockRollbackAndReload(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, e := s.Register(ctx, game.ClientInfo{Account: "好友事务甲", Password: "pw", Hostnum: 1})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Register(ctx, game.ClientInfo{Account: "好友事务乙", Password: "pw", Hostnum: 1})
	if e != nil {
		t.Fatal(e)
	}
	left, right := hex.EncodeToString(a.Avatars[0].OID), hex.EncodeToString(b.Avatars[0].OID)
	ids := []string{left, right}
	_, err := s.UpdateSocial(ctx, ids, func(rows map[string]*game.Avatar) error {
		rows[left].Progress.Social.Revision = 100
		rows[right].Progress.Social.Revision = 100
		return errors.New("双角色故障注入")
	})
	if err == nil {
		t.Fatal("故障没有回滚")
	}
	rows, err := s.SocialAvatars(ctx, game.SocialSearch{OIDs: ids, Limit: 2})
	if err != nil || len(rows) != 2 {
		t.Fatal(err)
	}
	for _, av := range rows {
		if av.Progress.Social.Revision != 0 {
			t.Fatal("双角色事务部分提交")
		}
	}
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(reverse bool) {
			defer wg.Done()
			order := ids
			if reverse {
				order = []string{right, left}
			}
			_, err := s.UpdateSocial(ctx, order, func(rows map[string]*game.Avatar) error {
				rows[left].Progress.Social.Revision++
				rows[right].Progress.Social.Revision++
				return nil
			})
			if err != nil {
				failures <- err
			}
		}(i%2 == 0)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	rows, err = New(s.pool).SocialAvatars(ctx, game.SocialSearch{OIDs: ids, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, av := range rows {
		if av.Progress.Social.Revision != 8 {
			t.Fatal("有序双行锁丢更新或未持久", av.Progress.Social.Revision)
		}
	}
	initialLevel := a.Avatars[0].Info.Level
	failures = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(social bool) {
			defer wg.Done()
			var e error
			if social {
				_, e = s.UpdateSocial(ctx, []string{right, left}, func(v map[string]*game.Avatar) error { v[left].Progress.AvatarLevel++; return nil })
			} else {
				_, e = s.UpdateProgress(ctx, a.Avatars[0].OID, func(p *game.Progress) error { p.AvatarLevel++; return nil })
			}
			if e != nil {
				failures <- e
			}
		}(i%2 == 0)
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	rows, err = New(s.pool).SocialAvatars(ctx, game.SocialSearch{OIDs: []string{left}, Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].Info.Level != initialLevel+8 || rows[0].Progress.AvatarLevel != initialLevel+8 {
		t.Fatal("普通与双角色事务等级锁顺序丢更新", err, rows)
	}
}

func pgSocialArgs(v ...any) []json.RawMessage {
	out := []json.RawMessage{}
	for _, x := range v {
		raw, _ := json.Marshal(x)
		out = append(out, raw)
	}
	return out
}
func pgSocialCall(t *testing.T, svc *game.Service, c *game.Connection, method string, args ...any) []game.Push {
	t.Helper()
	pushes, e := svc.Handle(context.Background(), c, method, pgSocialArgs(args...))
	if e != nil {
		t.Fatal(method, e)
	}
	for _, p := range pushes {
		if p.Method == "call_client_callback" {
			values := p.Args[1].([]any)
			if len(values) == 0 || values[0] != 0 {
				t.Fatal(method, "业务失败", values)
			}
		}
	}
	return pushes
}
func pgSocialOnline(t *testing.T, pushes []game.Push, peer string) bool {
	t.Helper()
	for _, p := range pushes {
		if p.Method != "client_prop_changed" {
			continue
		}
		v := p.Args[0].([]any)
		if v[0] != "friend_dict" {
			continue
		}
		for _, row := range v[1].(mobileproto.Map) {
			if row.Key == game.ObjectID(peer) {
				return row.Value.(map[string]any)["info"].(map[string]any)["online"].(bool)
			}
		}
	}
	t.Fatal("缺好友在线属性")
	return false
}

func TestPostgresSocialAssistBattleAtomicReconnectAndPresence(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	svc := pgSocialService(t, store)
	svc.Now = func() time.Time { return now }
	identities := []game.Identity{}
	conns := []*game.Connection{}
	for _, account := range []string{"真实助战使用者", "真实助战提供者"} {
		info := game.ClientInfo{Account: account, Password: "pw", Hostnum: 1}
		id, e := store.Register(ctx, info)
		if e != nil {
			t.Fatal(e)
		}
		identities = append(identities, id)
		_, e = store.UpdateProgress(ctx, id.Avatars[0].OID, func(p *game.Progress) error {
			p.UnlockSystems["assist"] = 1
			p.UnlockSystems["support"] = 1
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		c := pgSocialLogin(t, svc, info, id.Avatars[0].OID)
		conns = append(conns, c)
	}
	own, peer := identities[0].Avatars[0], identities[1].Avatars[0]
	ownID, peerID := hex.EncodeToString(own.OID), hex.EncodeToString(peer.OID)
	pgSocialCall(t, svc, conns[0], "apply_friend", 1, peerID, "真实库助战", 1, 0)
	pgSocialCall(t, svc, conns[1], "agree_apply_friend", 2, ownID)
	_, e := store.UpdateProgress(ctx, peer.OID, func(p *game.Progress) error { p.Cards[0].CardID = 3202; return nil })
	if e != nil {
		t.Fatal(e)
	}
	uuid := peer.Progress.Cards[0].UUID
	pgSocialCall(t, svc, conns[1], "set_assist_card", 3, uuid)
	pushes := pgSocialCall(t, svc, conns[0], "refresh_assist_use_times")
	if !pgSocialOnline(t, pushes, peerID) {
		t.Fatal("提供者真实在线未投影")
	}
	pgSocialCall(t, svc, conns[0], "select_assist_card", 4, uuid, map[string]any{"eid": peerID, "hostnum": 1, "nickname": "伪造昵称"})
	battle := "123456789012345678901234"
	_, e = store.UpdateProgress(ctx, own.OID, func(p *game.Progress) error {
		p.Battle = &game.BattleSession{UUID: battle, DungeonID: 100101, BattleID: 1101, Loaded: true}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	layout := map[string]any{"fighting_cards": []string{own.Progress.Cards[0].UUID, uuid}, "support_cards": []string{}, "storyline_cards": []string{}}
	pgSocialCall(t, svc, conns[0], "battle_fighting", layout)
	rows, e := New(store.pool).SocialAvatars(ctx, game.SocialSearch{OIDs: []string{ownID, peerID}, Limit: 2})
	if e != nil {
		t.Fatal(e)
	}
	var attack, defend game.Avatar
	for _, row := range rows {
		if hex.EncodeToString(row.OID) == ownID {
			attack = row
		} else {
			defend = row
		}
	}
	if len(attack.Progress.Cards) != 1 || attack.Progress.Social.Assist.Active[uuid] != 1 || defend.Progress.Social.Assist.Passive != 1 || attack.Progress.Social.Assist.Frozen == nil || attack.Progress.Social.Assist.Frozen.Profile.Nickname == "伪造昵称" {
		t.Fatal("真实库助战双角色消费、冻结或资产隔离失败")
	}
	before := attack.Progress.Materials[17].Count
	pgSocialCall(t, svc, conns[0], "battle_fighting", layout)
	rows, e = New(store.pool).SocialAvatars(ctx, game.SocialSearch{OIDs: []string{ownID}, Limit: 1})
	if e != nil || rows[0].Progress.Social.Assist.Active[uuid] != 1 || rows[0].Progress.Materials[17].Count != before {
		t.Fatal("真实库助战同UUID重复发奖", e)
	}
	svc.Detach(conns[1])
	pushes = pgSocialCall(t, svc, conns[0], "refresh_assist_use_times")
	if pgSocialOnline(t, pushes, peerID) {
		t.Fatal("提供者离线仍投影在线")
	}
	_, e = store.UpdateSocial(ctx, []string{ownID, peerID}, func(v map[string]*game.Avatar) error {
		v[ownID].Progress.Social.Assist.Active[uuid] = 99
		v[peerID].Progress.Social.Assist.Passive = 99
		return errors.New("真实库助战双方消费失败")
	})
	if e == nil {
		t.Fatal("真实库助战失败未回滚")
	}
	rows, e = New(store.pool).SocialAvatars(ctx, game.SocialSearch{OIDs: []string{ownID, peerID}, Limit: 2})
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range rows {
		if hex.EncodeToString(row.OID) == ownID {
			if row.Progress.Social.Assist.Active[uuid] != 1 {
				t.Fatal("主动次数部分提交")
			}
		} else if row.Progress.Social.Assist.Passive != 1 {
			t.Fatal("被动次数部分提交")
		}
	}
	svc.Detach(conns[0])
}

func TestPostgresSocialCommentVotesRollbackAndRank(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, e := s.Register(ctx, game.ClientInfo{Account: "评论作者", Password: "pw", Hostnum: 1})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Register(ctx, game.ClientInfo{Account: "评论读者", Password: "pw", Hostnum: 1})
	if e != nil {
		t.Fatal(e)
	}
	owner := a.Avatars[0]
	voter := b.Avatars[0]
	id := "123456789012345678901234"
	_, _, err := s.UpdateComment(ctx, owner.OID, id, func(av *game.Avatar, c *game.CardComment) error {
		if c.Likes == nil {
			return errors.New("新建评论回调没有可写投票表")
		}
		*c = game.CardComment{ID: id, Owner: hex.EncodeToString(av.OID), CardID: 4401, Content: "真实库评论", CreatedAt: 100, Likes: map[string]bool{}}
		av.Progress.Social.Revision++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	comments, err := New(s.pool).QueryComments(ctx, 4401, 0, 2)
	if err != nil || len(comments) != 1 || comments[0].Likes == nil || len(comments[0].Likes) != 0 {
		t.Fatal("零票评论重建Store后没有可写空投票表", err, comments)
	}
	commentOID, err := game.SocialOID(id)
	if err != nil {
		t.Fatal(err)
	}
	likeCount := func() int {
		t.Helper()
		var count int
		if err := s.pool.QueryRow(ctx, `SELECT like_count FROM card_comments WHERE comment_oid=$1`, commentOID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	voterBefore := pgHumanRead(t, s, voter.OID).Progress.Social.Revision
	_, _, err = s.UpdateComment(ctx, voter.OID, id, func(av *game.Avatar, c *game.CardComment) error {
		c.Likes[hex.EncodeToString(av.OID)] = true
		av.Progress.Social.Revision = 99
		return errors.New("点赞玩家故障注入")
	})
	if err == nil {
		t.Fatal("点赞故障没有失败")
	}
	comments, err = s.QueryComments(ctx, 4401, 0, 2)
	if err != nil || len(comments) != 1 || len(comments[0].Likes) != 0 || likeCount() != 0 || pgHumanRead(t, s, voter.OID).Progress.Social.Revision != voterBefore {
		t.Fatal("点赞故障部分提交", err, comments)
	}
	_, _, err = s.UpdateComment(ctx, voter.OID, id, func(av *game.Avatar, c *game.CardComment) error {
		c.Likes[hex.EncodeToString(av.OID)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	comments, err = New(s.pool).QueryComments(ctx, 4401, 0, 2)
	if err != nil || len(comments) != 1 || len(comments[0].Likes) != 1 || !comments[0].Likes[hex.EncodeToString(voter.OID)] || likeCount() != 1 {
		t.Fatal("评论点赞未复登保持", err)
	}
	// 两个独立Store同时读回同一评论加票，必须保留旧票及两份玩家进度。
	concurrent := []game.Avatar{}
	for _, name := range []string{"评论并发读者甲", "评论并发读者乙"} {
		identity, err := s.Register(ctx, game.ClientInfo{Account: name, Password: "pw", Hostnum: 1})
		if err != nil {
			t.Fatal(err)
		}
		concurrent = append(concurrent, identity.Avatars[0])
	}
	var wg sync.WaitGroup
	failures := make(chan error, len(concurrent))
	for _, actor := range concurrent {
		wg.Add(1)
		go func(actor game.Avatar) {
			defer wg.Done()
			_, _, err := New(s.pool).UpdateComment(ctx, actor.OID, id, func(av *game.Avatar, c *game.CardComment) error {
				c.Likes[hex.EncodeToString(av.OID)] = true
				av.Progress.Social.Revision++
				return nil
			})
			if err != nil {
				failures <- err
			}
		}(actor)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal("双Store并发投票失败", err)
	}
	comments, err = New(s.pool).QueryComments(ctx, 4401, 0, 2)
	if err != nil || len(comments) != 1 || len(comments[0].Likes) != 3 || likeCount() != 3 || !comments[0].Likes[hex.EncodeToString(voter.OID)] {
		t.Fatal("并发投票覆盖旧票或SQL计数不一致", err, comments)
	}
	for _, actor := range concurrent {
		if !comments[0].Likes[hex.EncodeToString(actor.OID)] || pgHumanRead(t, s, actor.OID).Progress.Social.Revision != 1 {
			t.Fatal("并发玩家投票/进度未整笔提交")
		}
	}
	_, _, err = New(s.pool).UpdateComment(ctx, concurrent[0].OID, id, func(av *game.Avatar, c *game.CardComment) error {
		delete(c.Likes, hex.EncodeToString(voter.OID))
		av.Progress.Social.Revision = 999
		return errors.New("已有多票评论与玩家进度整笔故障注入")
	})
	if err == nil {
		t.Fatal("多票评论整笔故障未拒绝")
	}
	comments, err = New(s.pool).QueryComments(ctx, 4401, 0, 2)
	if err != nil || len(comments) != 1 || len(comments[0].Likes) != 3 || likeCount() != 3 || !comments[0].Likes[hex.EncodeToString(voter.OID)] || pgHumanRead(t, s, concurrent[0].OID).Progress.Social.Revision != 1 {
		t.Fatal("已有投票或玩家进度未随评论整笔回滚", err, comments)
	}
	// 既有作者、幻书和标识不能由回调移交；错误必须连玩家资产一起回滚。
	actorBefore, _ := json.Marshal(pgHumanRead(t, s, concurrent[0].OID).Progress)
	commentBefore, _ := json.Marshal(comments[0])
	for _, changed := range []string{"作者", "幻书", "标识"} {
		_, _, err = New(s.pool).UpdateComment(ctx, concurrent[0].OID, id, func(av *game.Avatar, c *game.CardComment) error {
			av.Progress.Social.Revision = 999
			av.Progress.SelectedHeadID = 999
			switch changed {
			case "作者":
				c.Owner = hex.EncodeToString(concurrent[0].OID)
			case "幻书":
				c.CardID = 3202
			case "标识":
				c.ID = "234567890123456789012345"
			}
			return nil
		})
		if err == nil {
			t.Fatal("既有评论身份更改被接受", changed)
		}
		comments, err = New(s.pool).QueryComments(ctx, 4401, 0, 2)
		if err != nil || len(comments) != 1 {
			t.Fatal(err, comments)
		}
		actorAfter, _ := json.Marshal(pgHumanRead(t, s, concurrent[0].OID).Progress)
		commentAfter, _ := json.Marshal(comments[0])
		if !bytes.Equal(actorBefore, actorAfter) || !bytes.Equal(commentBefore, commentAfter) || likeCount() != 3 {
			t.Fatal("身份拒绝后评论或玩家进度部分提交", changed)
		}
	}
	// 私有测试行注入关系列/JSON身份不一致，使ON CONFLICT WHERE实际影响0行。
	if _, err = s.pool.Exec(ctx, `UPDATE card_comments SET card_id=3202 WHERE comment_oid=$1`, commentOID); err != nil {
		t.Fatal(err)
	}
	_, _, err = New(s.pool).UpdateComment(ctx, concurrent[0].OID, id, func(av *game.Avatar, c *game.CardComment) error {
		av.Progress.Social.Revision = 999
		av.Progress.SelectedHeadID = 999
		c.Likes[hex.EncodeToString(owner.OID)] = true
		return nil
	})
	if err == nil {
		t.Fatal("评论SQL影响0行却提交玩家进度")
	}
	actorAfter, _ := json.Marshal(pgHumanRead(t, s, concurrent[0].OID).Progress)
	comments, err = New(s.pool).QueryComments(ctx, 3202, 0, 2)
	if err != nil || len(comments) != 1 || len(comments[0].Likes) != 3 || !bytes.Equal(actorBefore, actorAfter) || likeCount() != 3 {
		t.Fatal("SQL影响0行后玩家或投票部分提交", err, comments)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE card_comments SET card_id=4401 WHERE comment_oid=$1`, commentOID); err != nil {
		t.Fatal(err)
	}
	ids := []string{hex.EncodeToString(owner.OID), hex.EncodeToString(voter.OID)}
	_, err = s.UpdateSocial(ctx, ids, func(rows map[string]*game.Avatar) error {
		rows[ids[0]].Progress.AsyncPvp.Score = 1000
		rows[ids[1]].Progress.AsyncPvp.Score = 1500
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rank, err := s.AsyncPvpRank(ctx, owner.OID, 0)
	if err != nil || rank != 2 {
		t.Fatal("真实跨服异步名次错误", rank, err)
	}
}

// 复现omitempty的真实持久化形状；无需PG，只验证存储解码后可写且旧票不丢。
func TestSocialCommentPersistentJSONVotesRemainWritable(t *testing.T) {
	empty, err := json.Marshal(game.CardComment{ID: "零票", Likes: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(empty, []byte(`"likes"`)) {
		t.Fatal("夹具未复现原生omitempty零票形状", string(empty))
	}
	for _, raw := range [][]byte{nil, empty, []byte(`{"likes":null}`), []byte(`{"likes":{}}`)} {
		var row game.CardComment
		if err := decodeSocialComment(raw, &row); err != nil {
			t.Fatal(err)
		}
		row.Likes["首票"] = true
		if len(row.Likes) != 1 {
			t.Fatal("缺失/null/空投票表回载不可写")
		}
	}
	raw, err := json.Marshal(game.CardComment{ID: "已有投票", Likes: map[string]bool{"原票": true, "另一票": true}})
	if err != nil {
		t.Fatal(err)
	}
	var row game.CardComment
	if err := decodeSocialComment(raw, &row); err != nil {
		t.Fatal(err)
	}
	row.Likes["新票"] = true
	if len(row.Likes) != 3 || !row.Likes["原票"] || !row.Likes["另一票"] {
		t.Fatal("持久解码归一化覆盖旧票")
	}
}
