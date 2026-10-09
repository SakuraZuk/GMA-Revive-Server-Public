package game

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestClientExtensionsMatchesColdAndRuntimeHotfix(t *testing.T) {
	t.Setenv("HS_REMAINING_GAMEPLAY_POLICY", "local-20261008-v1")
	t.Setenv("HS_SPECIAL_DRILL_REOPEN", "timber-12-20261008")
	previous := configuredActivities.Load()
	t.Cleanup(func() {
		if previous != nil {
			configuredActivities.Store(previous)
		} else {
			configuredActivities.Store(map[int]ActivitySchedule{})
		}
	})
	raw, err := os.ReadFile("../../deploy/data/activity-schedules.json")
	if err != nil {
		t.Fatal(err)
	}
	var schedules []ActivitySchedule
	if err = json.Unmarshal(raw, &schedules); err != nil {
		t.Fatal(err)
	}
	if err = SetActivitySchedules(schedules); err != nil {
		t.Fatal(err)
	}
	combined := clientExtensionsBody()
	var catalog struct {
		Startup map[string]string `json:"startup_scripts"`
		Runtime struct {
			Script string `json:"script"`
			Index  int    `json:"index"`
		} `json:"runtime"`
	}
	raw, err = os.ReadFile("../../deploy/data/hotfix.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	// 与生成器textwrap.indent一致：空行不缩进，运行期正文也必须在闭包内。
	runtimeLines := strings.Split(strings.TrimRight(combined, "\r\n"), "\n")
	for i, line := range runtimeLines {
		if strings.TrimSpace(line) != "" {
			runtimeLines[i] = "    " + line
		}
	}
	wantRuntime := "def _hs_runtime_install():\n" + strings.Join(runtimeLines, "\n") + "\n\n_hs_runtime_install()"
	if clientExtensionsScript() != wantRuntime {
		t.Fatal("游戏内热修必须与运行期闭包完全一致")
	}
	if catalog.Runtime.Script != wantRuntime || catalog.Runtime.Index != bridgeHotfixIndex {
		first := 0
		for first < len(wantRuntime) && first < len(catalog.Runtime.Script) && wantRuntime[first] == catalog.Runtime.Script[first] {
			first++
		}
		t.Fatalf("客户端活动/真人扩展与游戏服务器不一致：差异偏移%d，长度%d/%d，索引%d/%d", first, len(wantRuntime), len(catalog.Runtime.Script), bridgeHotfixIndex, catalog.Runtime.Index)
	}
	// 与生成器textwrap.indent一致：空行不缩进，完整扩展在安装闭包内。
	lines := strings.Split(strings.TrimRight(combined, "\r\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = "    " + line
		}
	}
	wrapped := "\n" + strings.Join(lines, "\n") + "\n\n_hs_install()"
	for version, script := range catalog.Startup {
		if !strings.HasSuffix(script, wrapped) {
			t.Fatal("冷启动缺少闭包内一致扩展", version)
		}
	}
	for _, marker := range []string{"# REVIVAL_BATTLE_BRIDGE", "# HS_ACTIVITY_SCHEDULES_BEGIN", "# HS_LOGIN_BATTLE_RECOVERY_BEGIN", "# HS_HUMAN_BRIDGE_BEGIN"} {
		if strings.Count(combined, marker) != 1 {
			t.Fatal("扩展重复安装", marker)
		}
	}
}
