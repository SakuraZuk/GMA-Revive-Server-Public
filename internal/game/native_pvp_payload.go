package game

import (
	"errors"
)

// 原生权威只读取共同房间的已冻结阵容；不从在线连接或客户端上行获取卡属性。
// 主战/援护原槽位单独传递，空槽不能因为生成roster而被压缩成另一套阵容。
func nativePvpPayload(r *HumanPvpRoom) (map[string]any, []any, error) {
	if r == nil || len(r.IDs) != 2 || r.IDs[0] == r.IDs[1] || !validObjectID(r.UUID) {
		return nil, nil, errors.New("原生PVP共同房间身份无效")
	}
	seen := map[string]bool{}
	roster := func(id string) ([]any, error) {
		player, exists := r.Players[id]
		if !exists || !player.Selected || player.Profile.EID != id {
			return nil, errors.New("原生PVP冻结玩家与房间参与者不符")
		}
		cards := cardMgrPropertiesWithRunes(player.Cards, player.Runes)
		result := []any{}
		for _, uuid := range player.Layout.team() {
			card, exists := cards[uuid]
			if !exists || !validObjectID(uuid) || seen[uuid] {
				return nil, errors.New("原生PVP卡UUID缺失、重复或跨双方冲突")
			}
			seen[uuid] = true
			result = append(result, card)
		}
		if len(result) == 0 || len(result) > 8 {
			return nil, errors.New("原生PVP冻结阵容数量不合法")
		}
		return result, nil
	}
	left, err := roster(r.IDs[0])
	if err != nil {
		return nil, nil, err
	}
	right, err := roster(r.IDs[1])
	if err != nil {
		return nil, nil, err
	}
	lp, rp := r.Players[r.IDs[0]], r.Players[r.IDs[1]]
	metadata := map[string]any{
		"avatar_id": r.IDs[0], "enemy_id": r.IDs[1], "battle_uuid": r.UUID,
		"dungeon_id": r.Dungeon, "battle_id": r.BattleID, "battle_type": 2,
		"enemy_auto": false, "enemy_roster": right,
		"fighting_card_uuids": lp.Layout.Fighting, "support_card_uuids": lp.Layout.Support,
		"enemy_fighting_card_uuids": rp.Layout.Fighting, "enemy_support_card_uuids": rp.Layout.Support,
	}
	return metadata, left, nil
}
