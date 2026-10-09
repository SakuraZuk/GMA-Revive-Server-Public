package game

import (
	"context"
	"reflect"
	"testing"
)

// 保留真实夹具存储事务；仅在SocialAvatars已取得旧快照后插入另一连接的提交。
type assistInterleaveAccounts struct {
	*FixtureAccounts
	own       string
	afterRead func() error
}

func (a *assistInterleaveAccounts) SocialAvatars(ctx context.Context, q SocialSearch) ([]Avatar, error) {
	rows, err := a.FixtureAccounts.SocialAvatars(ctx, q)
	if err == nil && len(q.OIDs) == 1 && q.OIDs[0] == a.own && a.afterRead != nil {
		hook := a.afterRead
		a.afterRead = nil
		if err = hook(); err != nil {
			return nil, err
		}
	}
	return rows, err
}

func TestStrangerAssistFrozenReadInterleaveRejectsNewBattle(t *testing.T) {
	for _, mode := range []struct {
		name             string
		started, consume bool
	}{
		{"已开战布阵恢复", true, false}, {"已开战消费重试", true, true}, {"已消费尚未开始恢复", false, true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			s, cs, base := socialTestWorld(t)
			ctx := context.Background()
			own, peer := hexOf(selectedOID(cs[0])), hexOf(selectedOID(cs[1]))
			provider := socialSaved(t, base, cs[1])
			external := provider.Progress.Cards[0].UUID
			original := "00112233445566778899aabb"
			_, err := base.UpdateProgress(ctx, selectedOID(cs[0]), func(p *Progress) error {
				p.Battle = &BattleSession{UUID: original, Loaded: true, Started: mode.started}
				p.Social.Assist.Frozen = &FriendAssist{Profile: socialProfile(provider), Card: provider.Progress.Cards[0], BattleUUID: original}
				p.Social.Assist.LastBattle = original
				p.Social.Assist.Active = map[string]int{external: 1}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			calls, fired := 0, false
			var interleaved Progress
			var providerAtInterleave Progress
			wrapper := &assistInterleaveAccounts{FixtureAccounts: base, own: own}
			wrapper.afterRead = func() error {
				fired = true
				providerAtInterleave = socialSaved(t, base, cs[1]).Progress
				var err error
				interleaved, err = base.UpdateProgress(ctx, selectedOID(cs[0]), func(p *Progress) error {
					p.Battle = &BattleSession{UUID: "00112233445566778899aabc", Loaded: true}
					p.Social.Assist.Frozen = nil
					p.Social.Assist.LastBattle = ""
					return nil
				})
				return err
			}
			s.Accounts = wrapper
			err = s.updateBattleFormation(ctx, cs[0], BattleLayout{Fighting: []string{external}}, 0, mode.consume, func(p *Progress) error { calls++; p.Battle.Started = true; return nil })
			if err == nil || !fired || calls != 0 {
				t.Fatal("另一连接换场后旧授权快照仍可启动新战", err, fired, calls)
			}
			actual := socialSaved(t, base, cs[0]).Progress
			if !reflect.DeepEqual(interleaved, actual) {
				t.Fatal("拒绝后新场/计次/资产仍被旧场事务修改")
			}
			afterPeer := socialSaved(t, base, cs[1])
			if !reflect.DeepEqual(providerAtInterleave, afterPeer.Progress) || hexOf(afterPeer.OID) != peer {
				t.Fatal("拒绝仍写提供者状态")
			}
		})
	}
}

func TestStrangerAssistOwnedReadInterleaveCannotBecomeExternal(t *testing.T) {
	s, cs, base := socialTestWorld(t)
	ctx := context.Background()
	own := hexOf(selectedOID(cs[0]))
	uuid := socialSaved(t, base, cs[0]).Progress.Cards[0].UUID
	calls, fired := 0, false
	var interleaved Progress
	wrapper := &assistInterleaveAccounts{FixtureAccounts: base, own: own}
	wrapper.afterRead = func() error {
		fired = true
		var err error
		interleaved, err = base.UpdateProgress(ctx, selectedOID(cs[0]), func(p *Progress) error {
			p.Cards = nil
			p.Social.Assist.Current = &FriendAssist{Card: Card{UUID: uuid, CardID: 4401}}
			return nil
		})
		return err
	}
	s.Accounts = wrapper
	err := s.updateBattleFormation(ctx, cs[0], BattleLayout{Fighting: []string{uuid}}, 0, true, func(p *Progress) error { calls++; p.Power.Value++; return nil })
	if err == nil || !fired || calls != 0 {
		t.Fatal("读后删卡能通过本人阵容免计次入口", err, fired, calls)
	}
	if !reflect.DeepEqual(interleaved, socialSaved(t, base, cs[0]).Progress) {
		t.Fatal("拒绝仍修改资产/选择/战斗状态")
	}
}
