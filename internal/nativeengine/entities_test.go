package nativeengine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

type entityCase struct {
	Native   []UnitSnapshot    `json:"native"`
	Client   []UnitSnapshot    `json:"client"`
	Left     bool              `json:"left"`
	Existing map[string]string `json:"existing,omitempty"`
	Frame    *CoordinateFrame  `json:"frame,omitempty"`
}

func copyUnits(t *testing.T, rows []UnitSnapshot) []UnitSnapshot {
	t.Helper()
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var cloned []UnitSnapshot
	if err = json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func entityCases(t *testing.T) []entityCase {
	t.Helper()
	native := []UnitSnapshot{
		{"eid": "n1", "role": 1601, "kind": "ally", "camp": 1, "native_type": 1, "hex": []int{0, 0, 0}},
		{"eid": "n2", "role": 2103, "kind": "enemy", "camp": 2, "native_type": 1, "hex": []int{2, -1, -1}},
		{"eid": "nf", "role": 0, "kind": "field", "camp": 0, "native_type": 4, "mf_id": 12, "hex": []int{1, -1, 0}},
		{"eid": "ns", "role": 2202, "kind": "support", "camp": 1, "native_type": 1, "hex": []int{5, -2, -3}},
	}
	client := []UnitSnapshot{
		{"eid": "73", "role": 2103, "kind": "ally", "camp": 1, "native_type": 1, "hex": []int{3, -1, -2}},
		{"eid": "5", "role": 2202, "kind": "support", "camp": 2, "native_type": 1, "hex": []int{20, -20, 0}},
		{"eid": "81", "role": 1601, "kind": "enemy", "camp": 2, "native_type": 1, "hex": []int{5, -2, -3}},
		{"eid": "6", "role": 0, "kind": "field", "camp": 0, "native_type": 4, "mf_id": 12, "hex": []int{7, -3, -4}},
	}
	base := entityCase{Native: native, Client: client, Left: false}
	cases := []entityCase{base}
	left := copyUnits(t, client)
	for _, raw := range left {
		camp, _ := identityInteger(raw["camp"])
		if camp == 1 || camp == 2 {
			raw["camp"] = 3 - camp
		}
	}
	cases = append(cases, entityCase{Native: native, Client: left, Left: true})
	// 同角色同阵营的歧义单位没有数字ID或当前位置兜底。
	dupe := copyUnits(t, native)
	twin := copyUnits(t, native[:1])[0]
	twin["eid"] = "n9"
	dupe = append(dupe, twin)
	cases = append(cases, entityCase{Native: dupe, Client: client, Left: false})
	dupeClient := copyUnits(t, client)
	twin = copyUnits(t, client[2:3])[0]
	twin["eid"] = "82"
	dupeClient = append(dupeClient, twin)
	cases = append(cases, entityCase{Native: native, Client: dupeClient, Left: false})
	// 重复EID即使另一行无效，也必须全部取消身份绑定。
	duplicateEID := append(copyUnits(t, client), UnitSnapshot{"eid": "81", "role": -1})
	cases = append(cases, entityCase{Native: native, Client: duplicateEID, Left: false})
	moved := copyUnits(t, client)
	moved[2]["hex"] = []int{100, -100, 0}
	cases = append(cases, entityCase{Native: native, Client: moved, Left: false, Existing: map[string]string{"n1": "81", "n2": "73", "ns": "5", "nf": "6"}})
	frame := &CoordinateFrame{Sign: -1, Offset: [3]int{5, -2, -3}, AnchorCount: 2}
	nSummon := append(copyUnits(t, native), UnitSnapshot{"eid": "n10", "role": 999, "kind": "ally", "camp": 1, "native_type": 1, "is_summon": true, "create_entity_eid": "n1", "create_skill_id": 160101, "origin_coord": []int{1, -1, 0}, "hex": []int{20, -20, 0}})
	cSummon := append(copyUnits(t, client), UnitSnapshot{"eid": "10", "role": 999, "kind": "enemy", "camp": 2, "native_type": 1, "is_summon": true, "create_entity_eid": "81", "create_skill_id": 160101, "origin_coord": []int{4, -1, -3}, "hex": []int{-20, 20, 0}})
	cases = append(cases, entityCase{Native: nSummon, Client: cSummon, Left: false, Frame: frame})
	cases = append(cases, entityCase{Native: nSummon, Client: cSummon, Left: false})
	wrongSpawn := copyUnits(t, cSummon)
	wrongSpawn[4]["origin_coord"] = []int{3, 0, -3}
	cases = append(cases, entityCase{Native: nSummon, Client: wrongSpawn, Left: false, Existing: map[string]string{"n10": "10"}, Frame: frame})
	missingCreator := copyUnits(t, cSummon)
	delete(missingCreator[4], "create_entity_eid")
	cases = append(cases, entityCase{Native: nSummon, Client: missingCreator, Left: false, Frame: frame})
	// 多层召唤物必须等待每一层创造者已证明，不猜断循环依赖。
	chainN := append(copyUnits(t, nSummon), UnitSnapshot{"eid": "n11", "role": 998, "kind": "ally", "camp": 1, "native_type": 1, "is_summon": true, "create_entity_eid": "n10", "create_skill_id": 999001, "origin_coord": []int{2, -2, 0}, "hex": []int{2, -2, 0}})
	chainC := append(copyUnits(t, cSummon), UnitSnapshot{"eid": "11", "role": 998, "kind": "enemy", "camp": 2, "native_type": 1, "is_summon": true, "create_entity_eid": "10", "create_skill_id": 999001, "origin_coord": []int{3, 0, -3}, "hex": []int{3, 0, -3}})
	cases = append(cases, entityCase{Native: chainN, Client: chainC, Left: false, Frame: frame})
	cycleN, cycleC := copyUnits(t, chainN), copyUnits(t, chainC)
	cycleN[4]["create_entity_eid"] = "n11"
	cycleC[4]["create_entity_eid"] = "11"
	cases = append(cases, entityCase{Native: cycleN, Client: cycleC, Left: false, Frame: frame})
	// 非注入的旧映射不能把两名角色压到同一个客户端实体。
	cases = append(cases, entityCase{Native: native, Client: client, Left: false, Existing: map[string]string{"n1": "81", "n2": "81"}})
	return cases
}

func TestNativeEntityBindingStableIdentityAndPinnedBoard(t *testing.T) {
	cases := entityCases(t)
	for i, c := range cases {
		before, _ := json.Marshal(c)
		mapping := BindEntities(c.Native, c.Client, c.Left, c.Existing, c.Frame)
		after, _ := json.Marshal(c)
		if !bytes.Equal(before, after) {
			t.Fatalf("案例%d修改了原始快照或旧映射", i)
		}
		seen := map[string]bool{}
		for _, client := range mapping {
			if seen[client] {
				t.Fatalf("案例%d映射不满足一对一", i)
			}
			seen[client] = true
		}
		if i == 0 {
			want := map[string]string{"n1": "81", "n2": "73", "nf": "6", "ns": "5"}
			if !reflect.DeepEqual(mapping, want) {
				t.Fatal("右端乱序身份映射错误", mapping)
			}
			f, err := DeriveCoordinateFrame(c.Native, c.Client, mapping)
			if err != nil || f.Sign != -1 || f.Offset != [3]int{5, -2, -3} || f.AnchorCount != 2 {
				t.Fatal("初始棋盘锚点变换错误", f, err)
			}
			for _, coord := range [][3]int{{0, 0, 0}, {3, -1, -2}, {-10, 5, 5}} {
				n, err := f.ClientToNative(coord)
				if err != nil {
					t.Fatal(err)
				}
				v, err := f.NativeToClient(n)
				if err != nil || v != coord {
					t.Fatal("棋盘双向变换不互逆", err)
				}
			}
		}
		if (i == 2 || i == 3 || i == 4) && mapping["n1"] != "" {
			t.Fatalf("歧义或重复ID案例%d仍然猜测了绑定", i)
		}
		if i == 5 && mapping["n1"] != "81" {
			t.Fatal("已移动单位丢失稳定身份")
		}
		if i == 6 && mapping["n10"] != "10" {
			t.Fatal("有完整出生证据的召唤物未被绑定")
		}
		if (i == 7 || i == 8 || i == 9 || i == 11) && mapping["n10"] != "" {
			t.Fatalf("案例%d召唤物没有完整证据仍被绑定", i)
		}
		if i == 10 && mapping["n11"] != "11" {
			t.Fatal("多层召唤物创造者依赖没有正确解开")
		}
	}
	base := cases[0]
	mapping := BindEntities(base.Native, base.Client, base.Left, nil, nil)
	moved := copyUnits(t, base.Client)
	moved[2]["hex"] = []int{100, -100, 0}
	if _, err := DeriveCoordinateFrame(base.Native, moved, mapping); err == nil {
		t.Fatal("已经独立移动的快照重新推出了错误棋盘变换")
	}
	if _, err := DeriveCoordinateFrame(base.Native, base.Client, map[string]string{"n1": "81"}); err == nil {
		t.Fatal("单锚点被当作坐标证据")
	}
}

func TestNativeEntityBindingMatchesAuthorUpdatedHelpers(t *testing.T) {
	python := os.Getenv("HS_NATIVE_PVP_TEST_PYTHON3")
	if python == "" {
		t.Skip("作者Python3纯函数对照只由专项显式启用")
	}
	base, err := filepath.Abs("../nativepvp")
	if err != nil {
		t.Fatal(err)
	}
	cases := entityCases(t)
	raw, _ := json.Marshal(cases)
	script := `import json,sys
sys.path.insert(0,sys.argv[1])
from sync_pvp_entities import build_native_eid_map
from sync_pvp_coordinates import CoordinateTransform,coordinate_transform,CoordinateMappingError
out=[]
for c in json.load(sys.stdin):
 f=c.get('frame')
 f=CoordinateTransform(f['sign'],tuple(f['offset']),f['anchor_count']) if f else None
 m=build_native_eid_map(c['native'],c['client'],client_is_left=c['left'],existing_map=c.get('existing'),coordinate_frame=f)
 try:
  d=coordinate_transform(c['native'],c['client'],m)
  frame=dict(sign=d.sign,offset=d.offset,anchor_count=d.anchor_count)
 except CoordinateMappingError:frame=None
 out.append(dict(mapping=m,frame=frame))
print(json.dumps(out))`
	cmd := exec.Command(python, "-c", script, base)
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal("作者新版纯函数执行失败", err, string(output))
	}
	var expected []struct {
		Mapping map[string]string `json:"mapping"`
		Frame   *CoordinateFrame  `json:"frame"`
	}
	if err = json.Unmarshal(output, &expected); err != nil || len(expected) != len(cases) {
		t.Fatal("作者对照结果结构错误", err, string(output))
	}
	for i, c := range cases {
		mapping := BindEntities(c.Native, c.Client, c.Left, c.Existing, c.Frame)
		if !reflect.DeepEqual(mapping, expected[i].Mapping) {
			t.Fatalf("案例%d与作者新版实体规则分歧：Go=%v 作者=%v", i, mapping, expected[i].Mapping)
		}
		frame, err := DeriveCoordinateFrame(c.Native, c.Client, mapping)
		if expected[i].Frame == nil {
			if err == nil {
				t.Fatalf("案例%d作者拒绝坐标映射而Go接受", i)
			}
		} else if err != nil || !reflect.DeepEqual(frame, expected[i].Frame) {
			t.Fatalf("案例%d与作者新版坐标规则分歧：%v %v", i, frame, err)
		}
	}
}
