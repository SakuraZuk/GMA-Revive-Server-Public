package game

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
)

// finished_guide 是界面引导的一参通知，与推进权威任务的guide_task_finished不同。
// 只保存原生目录中的已播放记录，不能据此发奖、通关或改变任务状态。
func (s *Service) finishedUIGuide(ctx context.Context, c *Connection, args []json.RawMessage) ([]Push, error) {
	var id int
	if c.phase != Playing || len(args) != 1 || json.Unmarshal(args[0], &id) != nil || id <= 0 {
		return nil, errors.New("界面引导完成需要玩家状态及单个正整数")
	}
	_, uiKnown := clientBaseline.UIGuides[id]
	_, taskKnown := clientBaseline.Guides[id]
	if !uiKnown && !taskKnown {
		return nil, errors.New("界面引导编号不在原生目录")
	}
	if containsInt(c.SelectedAvatarUnsafe().Progress.FinishedGuides, id) {
		return nil, nil
	}
	if err := s.updateProgress(ctx, c, func(p *Progress) error {
		if !containsInt(p.FinishedGuides, id) {
			p.FinishedGuides = append(p.FinishedGuides, id)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	av := c.SelectedAvatarUnsafe()
	log.Printf("界面引导已保存 uid=%d oid=%x guide=%d", av.UID, av.OID, id)
	// 原生客户端已本地append，无需重复推送或调用不存在的callback。
	return nil, nil
}

func redactDiagnostic(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, entry := range v {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "ticket") || strings.Contains(lower, "cookie") || strings.Contains(lower, "secret") {
				v[key] = "已隐藏"
			} else {
				v[key] = redactDiagnostic(entry)
			}
		}
	case []any:
		for i := range v {
			v[i] = redactDiagnostic(v[i])
		}
	case string:
		lower := strings.ToLower(v)
		if strings.Contains(lower, "password=") || strings.Contains(lower, "token=") || strings.Contains(lower, "ticket=") {
			return "含凭据内容已隐藏"
		}
	}
	return value
}

// client_sa_log 只用于诊断，限长限频且不修改角色状态，不提供资产权威。
func (s *Service) clientDiagnostic(c *Connection, args []json.RawMessage) ([]Push, error) {
	var key string
	var info any
	if c.phase != Playing || len(args) != 2 || len(args[0])+len(args[1]) > 16384 || json.Unmarshal(args[0], &key) != nil || len(key) > 128 || json.Unmarshal(args[1], &info) != nil {
		return nil, errors.New("客户端事件参数无效或过长")
	}
	now := s.Now().Unix()
	if now-c.diagnosticWindow >= 60 {
		c.diagnosticWindow, c.diagnosticCount = now, 0
	}
	if c.diagnosticCount >= 60 {
		return nil, nil
	}
	c.diagnosticCount++
	raw, _ := json.Marshal(redactDiagnostic(map[string]any{"事件": key, "内容": info}))
	av := c.SelectedAvatarUnsafe()
	log.Printf("客户端事件 uid=%d oid=%x %s", av.UID, av.OID, raw)
	return nil, nil
}
