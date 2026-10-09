package game

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestAvatarExpThresholdsAndTransactionRollback(t *testing.T) {
	now := time.Unix(1801840000, 0)
	p := NewProgress(1, now)
	p.Power.Value = 50
	if err := grantAvatarExp(&p, 1189, now); err != nil || p.AvatarLevel != 1 || p.AvatarExp != 1189 {
		t.Fatalf("阈值前:%+v %v", p, err)
	}
	if err := grantAvatarExp(&p, 1231, now); err != nil || p.AvatarLevel != 3 || p.AvatarExp != 0 || p.Power.Value != 50 || p.Power.Max != 102 {
		t.Fatalf("多级升级:%d/%d %v", p.AvatarLevel, p.AvatarExp, err)
	}
	av := Avatar{OID: NewAvatarOID(now), Info: DefaultAvatarInfo("测试"), Progress: p}
	av.Info.Level = 3
	a := NewFixtureAccounts(map[string]FixtureAccount{"经验测试": {Avatars: []Avatar{av}}})
	_, err := a.UpdateProgress(context.Background(), av.OID, func(p *Progress) error {
		if err := grantAvatarExp(p, 1269, now); err != nil {
			return err
		}
		return grantAvatarExp(p, math.MaxInt64, now)
	})
	if err == nil {
		t.Fatal("溢出未拒绝")
	}
	stored, err := a.AdminPlayer(context.Background(), av.OID)
	if err != nil || stored.Info.Level != 3 || stored.Progress.AvatarExp != 0 {
		t.Fatalf("事务未回滚:%+v %v", stored, err)
	}
}

func TestBattleCardExperienceFullAmountAndOwnedReceivers(t *testing.T) {
	now := time.Unix(1801840000, 0)
	p := NewProgress(20, now)
	a, b := newCard(4401, 1, now), newCard(4402, 1, now)
	p.Cards = []Card{a, b}
	receivers, err := grantCardBattleExp(&p, []string{a.UUID, b.UUID, a.UUID, newCardUUID()}, 104)
	if err != nil || len(receivers) != 2 || p.Cards[0].Level != 2 || p.Cards[1].Level != 2 || p.Cards[0].Exp != 0 || p.Materials[3].Count != 0 {
		t.Fatalf("经验未全额独立发放:%+v %v", p.Cards, err)
	}
	p.AvatarLevel = 1
	p.Cards[0].Level = 5
	p.Cards[0].Exp = 0
	if _, err = grantCardBattleExp(&p, []string{a.UUID}, 1000); err != nil || p.Cards[0].Level != 5 || p.Cards[0].Exp != 374 {
		t.Fatalf("馆主门槛:%+v %v", p.Cards[0], err)
	}
}
