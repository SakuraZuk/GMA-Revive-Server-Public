package game

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

//go:embed native_authority_bridge_script.py
var nativeAuthorityBridgeScript string

//go:embed activity_schedule_script.py
var activityScheduleScript string

//go:embed activity_metrics_script.py
var activityMetricsScript string

//go:embed activity_buffs_script.py
var activityBuffsScript string

//go:embed login_battle_recovery_script.py
var loginBattleRecoveryScript string

//go:embed client_diagnostic_script.py
var clientDiagnosticScript string

//go:embed summon_timer_script.py
var summonTimerScript string

//go:embed ui_callback_repair_script.py
var uiCallbackRepairScript string

// clientExtensionsScript 与冷启动热更采用相同组合。活动日程只读取服务器配置，
// 保留 rev12 原文；共享真人桥在普通副本中按 human_shared 开关保持原生行为。
func clientExtensionsBody() string {
	out := strings.ReplaceAll(battleBridgeScript, "\r\n", "\n")
	if configured := configuredActivities.Load(); configured != nil {
		entries := configured.(map[int]ActivitySchedule)
		rows := make([]ActivitySchedule, 0, len(entries))
		for _, row := range entries {
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		raw, err := json.Marshal(rows)
		if err != nil {
			panic("活动日程热更序列化失败")
		}
		compiled := strings.ReplaceAll(strings.ReplaceAll(activityScheduleScript, "\r\n", "\n"), "__HS_ACTIVITY_SCHEDULES_JSON__", strconv.Quote(string(raw)))
		approved := "False"
		if specialDrillReopenEnabled() {
			approved = "True"
		}
		compiled = strings.ReplaceAll(compiled, "__HS_SPECIAL_DRILL_REOPEN__", approved)
		out += "\n\n# HS_ACTIVITY_SCHEDULES_BEGIN\n" + compiled + "\n# HS_ACTIVITY_SCHEDULES_END\n"
	}
	out += "\n\n# HS_ACTIVITY_METRICS_BEGIN\n" + strings.ReplaceAll(activityMetricsScript, "\r\n", "\n") + "\n# HS_ACTIVITY_METRICS_END\n"
	out += "\n\n# HS_ACTIVITY_BUFFS_BEGIN\n" + strings.ReplaceAll(activityBuffsScript, "\r\n", "\n") + "\n# HS_ACTIVITY_BUFFS_END\n"
	out += "\n\n# HS_LOGIN_BATTLE_RECOVERY_BEGIN\n" + strings.ReplaceAll(loginBattleRecoveryScript, "\r\n", "\n") + "\n# HS_LOGIN_BATTLE_RECOVERY_END\n"
	out += "\n\n# HS_CLIENT_DIAGNOSTIC_BEGIN\n" + strings.ReplaceAll(clientDiagnosticScript, "\r\n", "\n") + "\n# HS_CLIENT_DIAGNOSTIC_END\n"
	out += "\n\n# HS_SUMMON_TIMER_BEGIN\n" + strings.ReplaceAll(summonTimerScript, "\r\n", "\n") + "\n# HS_SUMMON_TIMER_END\n"
	out += "\n\n# HS_UI_CALLBACK_REPAIR_BEGIN\n" + strings.ReplaceAll(uiCallbackRepairScript, "\r\n", "\n") + "\n# HS_UI_CALLBACK_REPAIR_END\n"
	out += "\n\n# HS_HUMAN_BRIDGE_BEGIN\n" + strings.ReplaceAll(humanBattleBridgeScript, "\r\n", "\n") + "\n# HS_HUMAN_BRIDGE_END\n"
	out += "\n\n# HS_NATIVE_RECORD_BEGIN\n" + strings.ReplaceAll(nativeRecordBridgeScript, "\r\n", "\n") + "\n# HS_NATIVE_RECORD_END\n"
	return out + "\n\n# HS_NATIVE_PVP_AUTHORITY_BEGIN\n" + strings.ReplaceAll(nativeAuthorityBridgeScript, "\r\n", "\n") + "\n# HS_NATIVE_PVP_AUTHORITY_END\n"
}

// 原生exec_hotfix_data分别提供globals/locals；外层函数闭包保留辅助函数引用。
// 运行期清单、游戏内推送和启动脚本必须采用同一正文和闭包语义。
func clientExtensionsScript() string {
	lines := strings.Split(strings.TrimRight(clientExtensionsBody(), "\r\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = "    " + line
		}
	}
	return "def _hs_runtime_install():\n" + strings.Join(lines, "\n") + "\n\n_hs_runtime_install()"
}
