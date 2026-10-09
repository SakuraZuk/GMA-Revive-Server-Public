package game

import (
	"fmt"
	"sort"
)

// instance_trigger/instance_event 数据驱动解释器（P0-4）。
// 表语义（out/client_catalogs/tables 两表 + out/dis/battle-do-trigger.asm 与
// battle-event-mgr.asm trigger.* 取证）：
//   - trigger_type: battle_start（开战即触发）/ turn_end（主体行动结束）/ turn_start（主体行动开始）
//     / entity_reduce（某阵营存活数降至 entity_remain_count）；
//   - 主体 = camp（该阵营任一实体）或 role_ids（该角色）；action_*_counter 是**触发器实例自身**
//     的计数器阈值（trigger.action_end/action_begin：self.counter += 1 后与 data 值 ==）；
//     setup_events 在触发器注册时把实例计数器归零，即"注册起第 N 次命中主体"才触发；
//   - entity_reduce 判定为存活数 <= remain（trigger.die 的 COMPARE_OP 1），且注册瞬间若条件
//     已成立立即结算（triggered_when_add + sequence_add_trigger_list）；
//   - 事件动作：set_ap/init_ap/role_ids 增援（服务器权威）、trigger_ids 注册新触发器、
//     battle_end= victory/fail（胜负）；storyline/battle_guide/位移/状态等演出由客户端本地执行。
// 10001 链实例（全表取证）：T1000101 battle_start → set_ap 4403=500 + 注册 T1000102；
// T1000102 camp2 首次行动结束 → 增援 202×3(camp2) + 107,107,106(camp1,init_ap=900) + 注册
// T1000103/T1000107；T1000103 camp1/4403 首次 turn_start → 注册 T1000104；T1000104 camp2 全灭
// → 第二波 202×4+201×2 + 注册 T1000105；T1000105 注册起 107 再行动 2 次 → battle_end=victory
// （教学真实胜利条件，非全灭判定；注册时 107 早已多次行动，必须按实例计数语义）。

// triggerRuntime 是解释器的持久化运行态（BattleSession.Triggers）。
type triggerRuntime struct {
	Armed      map[int]bool `json:"armed,omitempty"`       // 已注册待触发
	Fired      map[int]bool `json:"fired,omitempty"`       // 已触发（一次性消费）
	TriggerCnt map[int]int  `json:"trigger_cnt,omitempty"` // 每触发器自身计数（注册时归零）
}

func (b *BattleSession) ensureTriggers() {
	if b.Triggers == nil {
		b.Triggers = &triggerRuntime{}
	}
	// JSON omitempty 会丢弃空 map；反序列化回来的运行态需幂等补全。
	if b.Triggers.Armed == nil {
		b.Triggers.Armed = map[int]bool{}
	}
	if b.Triggers.Fired == nil {
		b.Triggers.Fired = map[int]bool{}
	}
	if b.Triggers.TriggerCnt == nil {
		b.Triggers.TriggerCnt = map[int]int{}
	}
}

// countAlive 统计某阵营存活实体数（trigger.count_entity_number 的存活集合）。
func (b *BattleSession) countAlive(camp int) int {
	count := 0
	for i := range b.Entities {
		if b.Entities[i].Camp == camp && b.Entities[i].Alive {
			count++
		}
	}
	return count
}

// checkTriggeredWhenAdd 复刻 trigger.triggered_when_add：entity_reduce 型触发器注册时
// 若条件已成立（存活数 <= remain），立即结算事件而非挂入等待。
func (b *BattleSession) checkTriggeredWhenAdd(def triggerDef) bool {
	if def.TriggerType != "entity_reduce" || def.Camp == nil || def.EntityRemainCount == nil {
		return false
	}
	return b.countAlive(*def.Camp) <= *def.EntityRemainCount
}

// armStartTriggers 注册 start_triggers 并立即结算 battle_start 型（开战时机）。
func (b *BattleSession) armStartTriggers() error {
	b.ensureTriggers()
	battle, ok := clientBaseline.Battles[b.BattleID]
	if !ok {
		return fmt.Errorf("战斗 %d 不在客户端基线", b.BattleID)
	}
	for _, tid := range battle.StartTriggers {
		def, ok := clientBaseline.WaveTriggers[tid]
		if !ok {
			return fmt.Errorf("触发器 %d 不在客户端基线", tid)
		}
		if def.TriggerType == "battle_start" {
			end, err := b.fireTrigger(def)
			if err != nil {
				return err
			}
			if end != "" {
				return fmt.Errorf("开战触发器 %d 不允许直接结束战斗 %s", tid, end)
			}
			continue
		}
		b.Triggers.Armed[tid] = true
		b.Triggers.TriggerCnt[tid] = 0
	}
	return nil
}

// onActionEnd 在行动结束（real_round_end 时机）评估 turn_end 触发器。
// 计数为触发器实例自身（注册时归零），每命中主体一次 +1，等于阈值触发。
// 返回非空 battle_end（victory/fail）表示事件链要求结束战斗。
func (b *BattleSession) onActionEnd(actor *battleEntity) (string, error) {
	if b.Triggers == nil || actor == nil {
		return "", nil
	}
	b.ensureTriggers()
	fired := make([]int, 0, 4)
	for tid, armed := range b.Triggers.Armed {
		if !armed {
			continue
		}
		def, ok := clientBaseline.WaveTriggers[tid]
		if !ok || def.TriggerType != "turn_end" || !turnSubjectMatches(def, actor) {
			continue
		}
		// 无 action_end_counter 的行不监听行动结束（setup_events 仅在字段非空时挂 action_end）。
		if def.ActionEndCounter == nil {
			continue
		}
		b.Triggers.TriggerCnt[tid]++
		if b.Triggers.TriggerCnt[tid] == *def.ActionEndCounter {
			fired = append(fired, tid)
		}
	}
	return b.fireAll(fired)
}

// onActionBegin 在 nextActor 选定后评估 turn_start 触发器。
func (b *BattleSession) onActionBegin(actor *battleEntity) (string, error) {
	if b.Triggers == nil || actor == nil {
		return "", nil
	}
	b.ensureTriggers()
	fired := make([]int, 0, 4)
	for tid, armed := range b.Triggers.Armed {
		if !armed {
			continue
		}
		def, ok := clientBaseline.WaveTriggers[tid]
		if !ok || def.TriggerType != "turn_start" || !turnSubjectMatches(def, actor) {
			continue
		}
		if def.ActionBeginCounter == nil {
			continue
		}
		b.Triggers.TriggerCnt[tid]++
		if b.Triggers.TriggerCnt[tid] == *def.ActionBeginCounter {
			fired = append(fired, tid)
		}
	}
	return b.fireAll(fired)
}

// onEntityReduce 在实体死亡或行动结算后评估 entity_reduce 触发器。
// 判定为存活数 <= entity_remain_count（反汇编 COMPARE_OP 1）。
func (b *BattleSession) onEntityReduce() (string, error) {
	if b.Triggers == nil {
		return "", nil
	}
	fired := make([]int, 0, 4)
	for tid, armed := range b.Triggers.Armed {
		if !armed {
			continue
		}
		def, ok := clientBaseline.WaveTriggers[tid]
		if !ok || def.TriggerType != "entity_reduce" || def.Camp == nil || def.EntityRemainCount == nil {
			continue
		}
		if b.countAlive(*def.Camp) <= *def.EntityRemainCount {
			fired = append(fired, tid)
		}
	}
	return b.fireAll(fired)
}

func turnSubjectMatches(def triggerDef, actor *battleEntity) bool {
	if len(def.RoleIDs) > 0 {
		for _, rid := range def.RoleIDs {
			if actor.RoleID == rid {
				return true
			}
		}
		return false
	}
	if def.Camp != nil {
		return actor.Camp == *def.Camp
	}
	return true
}

func (b *BattleSession) fireAll(fired []int) (string, error) {
	// 按触发器 id 升序保证事件处理序稳定（客户端事件表同序）。
	sort.Ints(fired)
	for _, tid := range fired {
		def, ok := clientBaseline.WaveTriggers[tid]
		if !ok {
			continue
		}
		end, err := b.fireTrigger(def)
		if err != nil {
			return "", err
		}
		if end != "" {
			return end, nil
		}
	}
	return "", nil
}

func (b *BattleSession) fireTrigger(def triggerDef) (string, error) {
	delete(b.Triggers.Armed, def.ID)
	b.Triggers.Fired[def.ID] = true
	for _, eventID := range def.TriggerEvents {
		end, err := b.runInstanceEvent(eventID)
		if err != nil {
			return "", err
		}
		if end != "" {
			return end, nil
		}
	}
	return "", nil
}

// runInstanceEvent 执行单个 instance_event 的服务器权威部分。
func (b *BattleSession) runInstanceEvent(eventID int) (string, error) {
	event, ok := clientBaseline.WaveEvents[eventID]
	if !ok {
		// 闭包收集范围外的行不参与服务器状态（导出已含全部闭包事件，正常不可达）。
		return "", nil
	}
	if event.SetAP != nil && len(event.RoleIDs) > 0 {
		for _, rid := range event.RoleIDs {
			for i := range b.Entities {
				if b.Entities[i].Camp == event.Camp && b.Entities[i].RoleID == rid {
					b.Entities[i].AP = *event.SetAP
				}
			}
		}
	} else if len(event.RoleIDs) > 0 {
		for _, roleID := range event.RoleIDs {
			if err := b.spawnEntity(event.Camp, roleID, event.InitAP); err != nil {
				return "", err
			}
		}
	}
	for _, tid := range event.TriggerIDs {
		def, ok := clientBaseline.WaveTriggers[tid]
		if !ok {
			continue
		}
		b.ensureTriggers()
		b.Triggers.Armed[tid] = true
		// 实例计数器在注册时归零（trigger.setup_events）。
		b.Triggers.TriggerCnt[tid] = 0
		// check_triggered_when_add：entity_reduce 型注册瞬间条件已成立立即结算。
		// 例如第一波先于"主角第二次行动开始"（T1000103→T1000104）被打光时，
		// 若不立即补触发，第二波增援与 T1000105 注册将永久错过。
		if b.checkTriggeredWhenAdd(def) {
			end, err := b.fireTrigger(def)
			if err != nil {
				return "", err
			}
			if end != "" {
				return end, nil
			}
		}
	}
	if event.BattleEnd != nil {
		return *event.BattleEnd, nil
	}
	return "", nil
}
