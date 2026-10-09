package game

import (
	"bytes"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// 只解释Android本版cPickle默认protocol0的数据操作码；从不调用反序列化对象或执行代码。
// ObjectId只允许原样例证明的copy_reg/bson构造与十二字节状态。
//
//go:embed native_record_catalog.json
var nativeRecordCatalogData []byte
var nativeRecordCatalog = func() struct {
	Methods     []string `json:"methods"`
	CustomTypes []string `json:"custom_types"`
} {
	var v struct {
		Methods     []string `json:"methods"`
		CustomTypes []string `json:"custom_types"`
	}
	if json.Unmarshal(nativeRecordCatalogData, &v) != nil || len(v.Methods) == 0 {
		panic("原生录像目录无效")
	}
	return v
}()

type pickleNode struct {
	kind   byte
	text   string
	number float64
	items  []*pickleNode
	keys   []*pickleNode
}
type nativePickle struct {
	raw   []byte
	at    int
	stack []*pickleNode
	memo  map[int]*pickleNode
	nodes int
}

var errNativePickle = errors.New("原生录像pickle结构无效或含未授权执行操作")

func (p *nativePickle) add(v *pickleNode) error {
	p.nodes++
	if p.nodes > 2000000 || len(p.stack) > 200000 {
		return errNativePickle
	}
	p.stack = append(p.stack, v)
	return nil
}
func (p *nativePickle) pop() (*pickleNode, error) {
	if len(p.stack) == 0 {
		return nil, errNativePickle
	}
	v := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	return v, nil
}
func (p *nativePickle) line() (string, error) {
	at := bytes.IndexByte(p.raw[p.at:], '\n')
	if at < 0 {
		return "", errNativePickle
	}
	v := string(p.raw[p.at : p.at+at])
	p.at += at + 1
	return v, nil
}
func (p *nativePickle) mark() ([]*pickleNode, error) {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].kind == '(' {
			v := append([]*pickleNode{}, p.stack[i+1:]...)
			p.stack = p.stack[:i]
			return v, nil
		}
	}
	return nil, errNativePickle
}
func pickleString(line string, quoted bool) (string, error) {
	if quoted {
		if len(line) < 2 || (line[0] != '\'' && line[0] != '"') || line[len(line)-1] != line[0] {
			return "", errNativePickle
		}
		line = line[1 : len(line)-1]
	}
	line = strings.ReplaceAll(line, "\\'", "'")
	line = strings.ReplaceAll(line, "\\\"", "\"")
	line = strings.ReplaceAll(line, "\"", "\\\"")
	v, err := strconv.Unquote("\"" + line + "\"")
	if err != nil {
		return "", errNativePickle
	}
	return v, nil
}
func parseNativePickle(raw []byte) (*pickleNode, error) {
	p := nativePickle{raw: raw, memo: map[int]*pickleNode{}}
	for p.at < len(raw) {
		op := raw[p.at]
		p.at++
		var v *pickleNode
		var err error
		switch op {
		case '(':
			v = &pickleNode{kind: '('}
		case 'N':
			v = &pickleNode{kind: 'N'}
		case 'I', 'L', 'F':
			line, e := p.line()
			if e != nil {
				return nil, e
			}
			if op == 'L' {
				line = strings.TrimSuffix(line, "L")
			}
			n, e := strconv.ParseFloat(line, 64)
			if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, errNativePickle
			}
			v = &pickleNode{kind: 'n', number: n}
		case 'S', 'V':
			line, e := p.line()
			if e != nil {
				return nil, e
			}
			s, e := pickleString(line, op == 'S')
			if e != nil {
				return nil, e
			}
			v = &pickleNode{kind: 's', text: s}
		case 'l', 't', 'd':
			items, e := p.mark()
			if e != nil {
				return nil, e
			}
			v = &pickleNode{kind: op}
			if op == 'd' {
				if len(items)%2 != 0 {
					return nil, errNativePickle
				}
				for i := 0; i < len(items); i += 2 {
					v.keys = append(v.keys, items[i])
					v.items = append(v.items, items[i+1])
				}
			} else {
				v.items = items
			}
		case 'a':
			item, e := p.pop()
			if e != nil || len(p.stack) == 0 {
				return nil, errNativePickle
			}
			list := p.stack[len(p.stack)-1]
			if list.kind != 'l' {
				return nil, errNativePickle
			}
			list.items = append(list.items, item)
			continue
		case 's':
			value, e := p.pop()
			if e != nil {
				return nil, e
			}
			key, e := p.pop()
			if e != nil || len(p.stack) == 0 {
				return nil, errNativePickle
			}
			dict := p.stack[len(p.stack)-1]
			if dict.kind != 'd' {
				return nil, errNativePickle
			}
			dict.keys = append(dict.keys, key)
			dict.items = append(dict.items, value)
			continue
		case 'p', 'g':
			line, e := p.line()
			if e != nil {
				return nil, e
			}
			index, e := strconv.Atoi(line)
			if e != nil || index < 0 || index > 2000000 {
				return nil, errNativePickle
			}
			if op == 'p' {
				if len(p.stack) == 0 {
					return nil, errNativePickle
				}
				p.memo[index] = p.stack[len(p.stack)-1]
				continue
			}
			v = p.memo[index]
			if v == nil {
				return nil, errNativePickle
			}
		case 'c':
			module, e := p.line()
			if e != nil {
				return nil, e
			}
			name, e := p.line()
			if e != nil {
				return nil, e
			}
			global := module + "." + name
			if global != "copy_reg._reconstructor" && global != "bson.objectid.ObjectId" && global != "__builtin__.object" {
				return nil, errNativePickle
			}
			v = &pickleNode{kind: 'c', text: global}
		case 'R':
			args, e := p.pop()
			if e != nil {
				return nil, e
			}
			fn, e := p.pop()
			if e != nil || fn.kind != 'c' || fn.text != "copy_reg._reconstructor" || args.kind != 't' || len(args.items) != 3 {
				return nil, errNativePickle
			}
			if args.items[0].kind != 'c' || args.items[0].text != "bson.objectid.ObjectId" || args.items[1].kind != 'c' || args.items[1].text != "__builtin__.object" || args.items[2].kind != 'N' {
				return nil, errNativePickle
			}
			v = &pickleNode{kind: 'o'}
		case 'b':
			state, e := p.pop()
			if e != nil || len(p.stack) == 0 {
				return nil, errNativePickle
			}
			obj := p.stack[len(p.stack)-1]
			if obj.kind != 'o' || obj.text != "" || state.kind != 's' || len(state.text) != 12 {
				return nil, errNativePickle
			}
			obj.text = state.text
			continue
		case '.':
			if len(p.stack) != 1 || p.at != len(raw) {
				return nil, errNativePickle
			}
			root := p.stack[0]
			if err := validatePickleData(root, map[*pickleNode]uint8{}, 0); err != nil {
				return nil, err
			}
			return root, nil
		default:
			return nil, fmt.Errorf("%w: opcode 0x%x", errNativePickle, op)
		}
		if err != nil {
			return nil, err
		}
		if err = p.add(v); err != nil {
			return nil, err
		}
	}
	return nil, errNativePickle
}
func nativeCustomType(name string) bool {
	for _, allowed := range nativeRecordCatalog.CustomTypes {
		if name == allowed {
			return true
		}
	}
	return false
}
func validatePickleData(v *pickleNode, seen map[*pickleNode]uint8, depth int) error {
	if v == nil || depth > 64 || seen[v] == 1 {
		return errNativePickle
	}
	if seen[v] == 2 {
		return nil
	}
	seen[v] = 1
	if v.kind == 'c' || v.kind == '(' || v.kind == 'o' && len(v.text) != 12 {
		return errNativePickle
	}
	if v.kind == 'd' {
		for i, key := range v.keys {
			if key.kind == 's' && key.text == "__custom_type" && (v.items[i].kind != 's' || !nativeCustomType(v.items[i].text)) {
				return errNativePickle
			}
		}
	}
	if (v.kind == 'l' || v.kind == 't') && len(v.items) >= 2 && v.items[len(v.items)-1].kind == 's' && v.items[len(v.items)-1].text == "__custom_type" {
		name := v.items[len(v.items)-2]
		if name.kind != 's' || !nativeCustomType(name.text) {
			return errNativePickle
		}
	}
	for _, item := range append(append([]*pickleNode{}, v.items...), v.keys...) {
		if err := validatePickleData(item, seen, depth+1); err != nil {
			return err
		}
	}
	seen[v] = 2
	return nil
}

func nativeRecordMethod(name string) bool {
	for _, allowed := range nativeRecordCatalog.Methods {
		if name == allowed {
			return true
		}
	}
	return false
}
func ValidateNativeRecordPickle(raw []byte, count int) error {
	root, err := parseNativePickle(raw)
	if err != nil {
		return err
	}
	if root.kind != 'l' || len(root.items) != count || count < 4 || count > 100000 {
		return errors.New("原生录像条目数不符")
	}
	last := 0.0
	found := map[string]bool{}
	for _, row := range root.items {
		if row.kind != 't' || len(row.items) != 4 {
			return errNativePickle
		}
		t, method, args, kwds := row.items[0], row.items[1], row.items[2], row.items[3]
		if t.kind != 'n' || t.number < last || method.kind != 's' || !nativeRecordMethod(method.text) || (args.kind != 'l' && args.kind != 't') || kwds.kind != 'd' {
			return errors.New("原生录像时间、方法或参数结构无效")
		}
		last = t.number
		found[method.text] = true
	}
	if !found["set_last_fighting_cards"] || !found["prepare"] || !found["add_fighting_cards"] || !found["battle_end_notice"] {
		return errors.New("原生录像缺少准备、阵容或真实结束通知，未截断补造")
	}
	return nil
}

func validateNativeRecordOwnership(raw []byte, owner string, receipt AsyncPvpRecord) error {
	root, err := parseNativePickle(raw)
	if err != nil {
		return err
	}
	id := func(v *pickleNode) string {
		if v.kind == 'o' {
			return hex.EncodeToString([]byte(v.text))
		}
		if v.kind == 's' && validObjectID(v.text) {
			return v.text
		}
		return ""
	}
	prepared, ended := false, false
	for _, row := range root.items {
		method, args := row.items[1].text, row.items[2]
		if method == "prepare" {
			if len(args.items) < 4 || args.items[3].kind != 'n' || args.items[3].number != float64(asyncRule().DungeonID) {
				return errors.New("原生录像副本归属不符")
			}
			players := args.items[0]
			if players.kind != 'l' && players.kind != 't' {
				return errNativePickle
			}
			found := map[string]bool{}
			for _, player := range players.items {
				found[id(player)] = true
			}
			if !found[owner] || !found[receipt.EnemyInfo.EID] {
				return errors.New("原生录像参与者与冻结异步记录不符")
			}
			prepared = true
		}
		if method == "battle_end_notice" {
			if len(args.items) < 1 {
				return errNativePickle
			}
			winners := args.items[0]
			if (winners.kind != 'l' && winners.kind != 't') || len(winners.items) != 1 {
				return errors.New("原生录像缺少唯一原生胜方")
			}
			winner := receipt.EnemyInfo.EID
			if receipt.Win {
				winner = owner
			}
			if id(winners.items[0]) != winner {
				return errors.New("原生录像胜方与已结算结果不符")
			}
			ended = true
		}
	}
	if !prepared || !ended {
		return errors.New("原生录像未完整覆盖冻结会话与结束")
	}
	return nil
}
