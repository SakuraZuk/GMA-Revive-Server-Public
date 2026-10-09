package game

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
)

// getRecordResult 取 Android RECORD_TYPE 的最近记录，只有当前鉴权角色可以查询。
// 1为同步结果、2为异步防御结果、8为异步攻击结果；4/16为录像类型，不能以截断观察日志冒充录像。
func (s *Service) getRecordResult(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing || len(args) != 1 {
		return nil, errors.New("战斗记录请求需要玩家状态及记录类型")
	}
	var kind int
	if json.Unmarshal(args[0], &kind) != nil {
		return nil, errors.New("战斗记录类型无效")
	}
	store, ok := s.Accounts.(SocialAccounts)
	if !ok {
		return nil, errors.New("账号存储不支持记录查询")
	}
	rows, err := store.SocialAvatars(ctx, SocialSearch{OIDs: []string{hexOf(selectedOID(c))}, Limit: 1})
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, errors.New("没有当前角色记录")
	}
	p := rows[0].Progress
	// 保存此次原生列表顺序供start_record_battle相同index解析。
	for i, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			av.Progress = p
			c.identity.Avatars[i] = av
		}
	}
	records := []any{}
	switch kind {
	case socialCatalog.Constants["RECORD_TYPE_SYNC_PVP_RESULT"]:
		list := append([]SyncPvpRecord{}, p.SyncPvpRecords...)
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
		for _, record := range list {
			if record.OwnInfo.EID == "" {
				continue
			}
			if record.OwnInfo.EID != hexOf(selectedOID(c)) {
				return nil, errors.New("同步记录归属不符")
			}
			records = append(records, syncPvpRecordWire(record))
		}
	case socialCatalog.Constants["RECORD_TYPE_ASYN_PVP_DEFENSE_RESULT"]:
		list := sortedAsyncRecords(p.AsyncPvp.DefenceRecords)
		if c.nativeRecordLists == nil {
			c.nativeRecordLists = map[int][]string{}
		}
		c.nativeRecordLists[4] = nil
		for _, record := range list {
			c.nativeRecordLists[4] = append(c.nativeRecordLists[4], record.UUID)
			records = append(records, map[string]any{"record": record.wire()})
		}
	case socialCatalog.Constants["RECORD_TYPE_ASYN_PVP_ATTACK_RESULT"]:
		list := sortedAsyncRecords(p.AsyncPvp.AttackRecords)
		if c.nativeRecordLists == nil {
			c.nativeRecordLists = map[int][]string{}
		}
		c.nativeRecordLists[16] = nil
		for _, record := range list {
			c.nativeRecordLists[16] = append(c.nativeRecordLists[16], record.UUID)
			records = append(records, map[string]any{"record": record.wire()})
		}
	default:
		return nil, errors.New("该记录类型没有完整原生录像来源")
	}
	return []Push{push("Avatar", "update_record_result", kind, records)}, nil
}

func sortedAsyncRecords(records []AsyncPvpRecord) []AsyncPvpRecord {
	list := append([]AsyncPvpRecord{}, records...)
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Time != list[j].Time {
			return list[i].Time > list[j].Time
		}
		return list[i].UUID < list[j].UUID
	})
	return list
}
func syncPvpRecordWire(r SyncPvpRecord) map[string]any {
	winner := r.EnemyInfo.EID
	if r.Outcome == "win" {
		winner = r.OwnInfo.EID
	}
	return map[string]any{"record_time": r.CreatedAt, "battle_uuid": ObjectID(r.BattleUUID), "winner_eid": ObjectID(winner), "score": r.Score, "pvp_info": map[string]any{r.OwnInfo.EID: map[string]any{"avatar_info": r.OwnInfo.wire(), "card_record": r.OwnTeam, "cards": r.OwnCards}, r.EnemyInfo.EID: map[string]any{"avatar_info": r.EnemyInfo.wire(), "card_record": r.EnemyTeam, "cards": r.EnemyCards}}}
}
