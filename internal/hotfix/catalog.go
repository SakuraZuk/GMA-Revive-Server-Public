// Package hotfix 管理启动期和运行期两个独立的热修通道，不执行脚本。
package hotfix

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"unicode/utf8"
)

type Runtime struct {
	Index  int    `json:"index"`
	Script string `json:"script"`
}

type Release struct {
	StartupScripts map[string]string `json:"startup_scripts"`
	Runtime        Runtime           `json:"runtime"`
}

// Catalog 发布不可变快照，同进程内禁止运行期索引倒退或同号换代码。
type Catalog struct {
	mu      sync.RWMutex
	current Release
	loaded  bool
}

func (c *Catalog) Load(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !utf8.Valid(body) {
		return errors.New("热修配置必须使用 UTF-8 编码")
	}
	var next Release
	if err = json.Unmarshal(body, &next); err != nil {
		return err
	}
	if next.StartupScripts == nil || next.Runtime.Index < 0 || (next.Runtime.Index > 0 && next.Runtime.Script == "") {
		return errors.New("热修配置缺少启动期映射或运行期索引、源码不合法")
	}
	for version := range next.StartupScripts {
		if version == "" {
			return errors.New("启动期版本键不能为空")
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded && (next.Runtime.Index < c.current.Runtime.Index || (next.Runtime.Index == c.current.Runtime.Index && next.Runtime.Script != c.current.Runtime.Script)) {
		return errors.New("运行期热修不得回退索引或在同一索引修改源码")
	}
	c.current, c.loaded = next, true
	return nil
}

func (c *Catalog) Startup() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	body, _ := json.Marshal(c.current.StartupScripts)
	return base64.StdEncoding.EncodeToString(body)
}

// Query 无更新时回客户端原索引和空串，避免要求客户端降级。
func (c *Catalog) Query(localIndex int) Runtime {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.current.Runtime.Index > localIndex {
		return c.current.Runtime
	}
	return Runtime{Index: localIndex, Script: ""}
}
