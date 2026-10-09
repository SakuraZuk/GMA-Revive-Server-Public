package game

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hs-server/internal/mobileproto"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 内部状态保留旧存档的0/1/2；原生MAIL_STATE为1/2/3，在wire边界转换。
const (
	MailUnread = iota
	MailUnclaimed
	MailFinal
)

type Mail struct {
	MID               int             `json:"mid"`
	UUID              string          `json:"uuid,omitempty"`
	Title             string          `json:"title"`
	Content           string          `json:"content"`
	Sender            string          `json:"sender,omitempty"`
	State             int             `json:"state"`
	Attachments       map[int]int64   `json:"attachments,omitempty"`
	SourceAttachments map[int]int64   `json:"source_attachments,omitempty"`
	RewardsFrozen     bool            `json:"rewards_frozen,omitempty"`
	Cards             []Card          `json:"card_attachments,omitempty"`
	Runes             map[string]Rune `json:"rune_attachments,omitempty"`
	CreatedAt         int64           `json:"created_at"`
	ModifiedAt        int64           `json:"modified_at,omitempty"`
	ExpiresAt         int64           `json:"expires_at,omitempty"`
}

func (m Mail) hasAttachments() bool { return len(m.Attachments)+len(m.Cards)+len(m.Runes) > 0 }

func mailAttachmentBox(m Mail) map[string]any {
	materials := mobileproto.Map{}
	ids := []int{}
	for id := range m.Attachments {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		materials = append(materials, mobileproto.Pair{Key: id, Value: m.Attachments[id]})
	}
	p := Progress{Cards: m.Cards}
	uuids := []string{}
	for _, card := range m.Cards {
		uuids = append(uuids, card.UUID)
	}
	return map[string]any{"__custom_type": "box.box", "materials": materials, "cards": cardListWire(&p, uuids), "runes": runeMgrProperties(m.Runes)}
}

func validateMailAssets(m Mail) error {
	return validateMailAssetsWithStoredGrade(m, false)
}

// 只在领取已持久邮件时保留旧版本签发的Grade1初始模板；新签发必须使用原生Grade0。
// 兼容不改附件或库存的既有品阶，其他资产、成长字段和容量仍按原校验执行。
func validateMailAssetsWithStoredGrade(m Mail, allowStoredGradeOne bool) error {
	if len(m.Cards) > 100 || len(m.Runes) > 100 {
		return errors.New("邮件卡或契印附件过多")
	}
	seen := map[string]bool{}
	for _, card := range m.Cards {
		if !validObjectID(card.UUID) || card.UUID != strings.ToLower(card.UUID) || seen[card.UUID] {
			return errors.New("邮件幻书编号无效或重复")
		}
		seen[strings.ToLower(card.UUID)] = true
		gradeOK := card.Grade == gradeByLevel(1) || (allowStoredGradeOne && card.Grade == 1)
		// 兼容旧附件的零装帧，并接受本卡原生默认装帧；付费外观仍不可借邮件签发。
		dressOK := card.Dress == 0 || card.Dress == androidCardAppearances[card.CardID].DefaultDress
		if _, ok := androidOath.Cards[card.CardID]; !ok || card.Level != 1 || !gradeOK || card.Exp != 0 || card.RingID != 0 || card.IsUpdateLevelOne || card.EnhanceCount != 0 || card.Awakened != 0 || !dressOK || card.Lock != 0 || card.SkillEnhanceCount != 0 || card.SupportSkillLevel != 1 || card.Time < 0 {
			return errors.New("邮件只接受原生初始幻书模板")
		}
	}
	tables, err := loadRuneTables()
	if err != nil {
		return err
	}
	for id, r := range m.Runes {
		if !validObjectID(id) || id != r.UUID || id != strings.ToLower(id) || r.CardUUID != "" {
			return errors.New("邮件契印编号或归属无效")
		}
		if err := validateRuneForEquip(r, tables); err != nil {
			return err
		}
	}
	return nil
}

func validMailUUID(uuid string) bool {
	b, err := hex.DecodeString(uuid)
	return err == nil && len(b) == 12
}

func (m Mail) oid() ObjectID {
	if validMailUUID(m.UUID) {
		return ObjectID(strings.ToLower(m.UUID))
	}
	// 旧整数编号稳定映射，无需覆盖旧存档或丢失附件。
	return ObjectID(fmt.Sprintf("0000000000000000%08x", uint32(m.MID)))
}
func (m Mail) expired(now int64) bool { return m.ExpiresAt > 0 && now >= m.ExpiresAt }
func mailText(text string) map[string]any {
	return map[string]any{"template": "%s", "args": []any{text}}
}

func mailProperties(mails map[int]Mail, now int64) mobileproto.Map {
	out := mobileproto.Map{}
	ids := make([]int, 0, len(mails))
	for id := range mails {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		m := mails[id]
		if m.expired(now) {
			continue
		}
		out = append(out, mobileproto.Pair{Key: m.oid(), Value: map[string]any{
			"__custom_type": "short_mail.short_mail", "mid": m.oid(), "source_eid": nil,
			"inner_title": mailText(m.Title), "state": m.State + 1,
			"create_time": m.CreatedAt, "last_modified_time": m.ModifiedAt, "expired_time": m.ExpiresAt,
			"has_attachemnt": m.hasAttachments(), "renew_mid": nil,
		}})
	}
	return out
}

func mailContent(m Mail, target ObjectID) map[string]any {
	return map[string]any{"__custom_type": "raw_mail.raw_mail", "_id": m.oid(), "source_eid": nil, "target_eid": target,
		"inner_title": mailText(m.Title), "inner_content": mailText(m.Content), "sender_name": m.Sender,
		"create_time": m.CreatedAt, "expired_time": m.ExpiresAt, "exclude_time": 0, "channel": "server",
		"attachment":     mailAttachmentBox(m),
		"has_attachemnt": m.hasAttachments(), "card_dict": cardMgrProperties(m.Cards), "rune_dict": runeMgrProperties(m.Runes)}
}

func itoa(v int) string { return strconv.Itoa(v) }

func (s *Service) IssueMail(ctx context.Context, c *Connection, m Mail) (resultErr error) {
	c.mu.Lock()
	defer func() {
		p := CloneProgress(c.SelectedAvatarUnsafe().Progress)
		oid := append([]byte(nil), selectedOID(c)...)
		c.mu.Unlock()
		if resultErr == nil {
			s.publishMailbox(oid, p)
		}
	}()
	if c.phase != Playing {
		return errors.New("在线发件需要玩家会话")
	}
	return s.updateProgress(ctx, c, func(p *Progress) error { return s.prepareAndInsertMail(p, m) })
}

func (s *Service) prepareAndInsertMail(p *Progress, m Mail) error {
	if m.MID <= 0 || strings.TrimSpace(m.Title) == "" || len([]rune(m.Title)) > 80 || len([]rune(m.Content)) > 4000 || len([]rune(m.Sender)) > 40 || len(m.Attachments) > 20 {
		return errors.New("邮件字段无效")
	}
	if m.UUID != "" && !validMailUUID(m.UUID) {
		return errors.New("邮件UUID无效")
	}
	if m.RewardsFrozen || len(m.SourceAttachments) != 0 {
		return errors.New("邮件随机奖励收据只能由服务端生成")
	}
	for id, n := range m.Attachments {
		if err := validateMailItemDefinition(id, n, s.Now()); err != nil {
			return err
		}
	}
	if err := validateMailAssets(m); err != nil {
		return err
	}
	m.State = MailUnread
	if m.CreatedAt == 0 {
		m.CreatedAt = s.Now().Unix()
	}
	if m.CreatedAt < 0 || m.CreatedAt > s.Now().Unix() {
		return errors.New("邮件创建时间无效")
	}
	if m.ExpiresAt != 0 && m.ExpiresAt <= s.Now().Unix() {
		return errors.New("邮件有效期已结束")
	}
	if m.UUID == "" {
		m.UUID = newBattleUUID(s.Now())
	}
	if m.Sender == "" {
		m.Sender = "管理员"
	}
	if err := freezeMailRandomRewards(*p, &m, s.Now()); err != nil {
		return err
	}
	m.ModifiedAt = s.Now().Unix()
	return insertMail(p, m)
}

// 固定附件采用与商店/结算相同的原生资产解释器；随机礼盒需另有冻结奖励收据，
// 因而不在固定附件中开放。框/装束/家具绝不作为普通库存材料保存。
func validateMailItemDefinition(id int, n int64, now time.Time) error {
	rule, known := androidShop.Materials[id]
	if !known || n <= 0 {
		return errors.New("邮件附件编号或数量无效")
	}
	switch rule.Type {
	case 1:
		if id != 1 || n > math.MaxInt32 {
			return errors.New("邮件体力附件无效")
		}
	case 2:
		if (id != 2 && id != 4) || (id == 2 && n > math.MaxInt32) {
			return errors.New("邮件经验附件未支持")
		}
	case 3, 4:
		return nil
	case 7:
		if rule.Sub != 1 || rule.Target <= 0 || n > 10000 {
			return errors.New("邮件只支持原生即时展开礼盒")
		}
		if _, known := androidShop.Bonuses[rule.Target]; !known {
			return errors.New("邮件礼盒奖励目录缺失")
		}
	case 6, 8, 9, 13:
		// 使用空角色验证模板；不提前把附件发给收件人或启动限时资源计时。
		var definition Progress
		return grantNativeItem(&definition, id, n, 1, now, map[int]int64{}, new([]string), 0)
	default:
		return errors.New("此邮件附件类型尚未取证")
	}
	return nil
}

func insertMail(p *Progress, m Mail) error {
	if p.ShortMailInfo == nil {
		p.ShortMailInfo = map[int]Mail{}
	}
	if len(p.ShortMailInfo) >= 500 {
		return errors.New("邮箱已满")
	}
	if _, ok := p.ShortMailInfo[m.MID]; ok {
		return errors.New("邮件编号已存在")
	}
	if err := validateMailboxAssetIDs(p, m); err != nil {
		return err
	}
	for _, old := range p.ShortMailInfo {
		if old.oid() == m.oid() {
			return errors.New("邮件UUID已存在")
		}
	}
	p.ShortMailInfo[m.MID] = m
	return advanceMailRevision(p)
}

func findMail(p *Progress, raw json.RawMessage) (int, Mail, error) {
	var uuid string
	if json.Unmarshal(raw, &uuid) == nil {
		if !validMailUUID(uuid) {
			return 0, Mail{}, errors.New("邮件编号无效")
		}
		for id, m := range p.ShortMailInfo {
			if string(m.oid()) == strings.ToLower(uuid) {
				return id, m, nil
			}
		}
	} else {
		// 兼容旧诊断入口；原生客户端发送ObjectID转换后的十六进制串。
		var id int
		if json.Unmarshal(raw, &id) == nil {
			if m, ok := p.ShortMailInfo[id]; ok {
				return id, m, nil
			}
		}
	}
	return 0, Mail{}, errors.New("邮件不存在")
}

func claimMail(p *Progress, m *Mail, now int64) error {
	if m.expired(now) {
		return errors.New("邮件已过期")
	}
	if m.State == MailFinal || !m.hasAttachments() {
		return errors.New("没有可领取附件")
	}
	if err := validateMailAssetsWithStoredGrade(*m, true); err != nil {
		return err
	}
	if len(m.Cards) > androidCompose.MaxCards-len(p.Cards) {
		return runeReject("RET_CARD_COUNT_REACH_MAX", "邮件幻书超出背包容量")
	}
	tables, err := loadRuneTables()
	if err != nil {
		return err
	}
	if len(m.Runes) > tables.Tables.Limits.Max-len(p.Runes) {
		return runeReject("RET_RUNE_MAX_COUNT_EXCEED", "邮件契印超出背包容量")
	}
	for _, card := range m.Cards {
		if _, owned := findCard(p, card.UUID); owned != nil {
			return errors.New("邮件幻书UUID与库存冲突")
		}
	}
	for id := range m.Runes {
		if _, owned := p.Runes[id]; owned {
			return errors.New("邮件契印UUID与库存冲突")
		}
	}
	candidate := CloneProgress(*p)
	ids := make([]int, 0, len(m.Attachments))
	for id := range m.Attachments {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		n := m.Attachments[id]
		if err := validateMailItemDefinition(id, n, time.Unix(now, 0)); err != nil {
			return err
		}
		if androidShop.Materials[id].Type == 7 {
			return errors.New("邮件随机奖励缺少签发时冻结收据")
		}
		if err := grantNativeItem(&candidate, id, n, candidate.AvatarLevel, time.Unix(now, 0), map[int]int64{}, new([]string), 0); err != nil {
			return err
		}
	}
	for _, card := range m.Cards {
		if len(candidate.Cards) >= androidCompose.MaxCards {
			return runeReject("RET_CARD_COUNT_REACH_MAX", "邮件全部幻书超出背包容量")
		}
		appendOwnedCard(&candidate, card)
	}
	if len(m.Runes) > 0 && candidate.Runes == nil {
		candidate.Runes = map[string]Rune{}
	}
	for id, r := range m.Runes {
		candidate.Runes[id] = r
	}
	*p = candidate
	m.State = MailFinal
	m.ModifiedAt = now
	return nil
}

func (s *Service) mailRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	if c.phase != Playing {
		return nil, errors.New("邮件操作需要玩家状态")
	}
	now := s.Now().Unix()
	if method == "query_mail_content" {
		if len(args) != 1 {
			return nil, errors.New("邮件查询需要邮件编号")
		}
		p := c.SelectedAvatarUnsafe().Progress
		_, m, err := findMail(&p, args[0])
		if err != nil {
			return nil, err
		}
		if m.expired(now) {
			return nil, errors.New("邮件已过期")
		}
		return []Push{push("Avatar", "on_query_mail_content", m.oid(), mailContent(m, ObjectID(hexOf(selectedOID(c)))))}, nil
	}
	want := 2
	if method == "receive_all_attachments" {
		want = 1
	}
	if len(args) != want {
		return nil, errors.New("邮件参数数量无效")
	}
	cb, ok := callbackArg(args)
	if !ok {
		return nil, errors.New("邮件回调编号无效")
	}
	changed := false
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		if method == "receive_all_attachments" {
			for id, m := range p.ShortMailInfo {
				if m.State == MailFinal || !m.hasAttachments() || m.expired(now) {
					continue
				}
				if err := claimMail(p, &m, now); err != nil {
					return err
				}
				p.ShortMailInfo[id] = m
				if err := advanceMailRevision(p); err != nil {
					return err
				}
				changed = true
			}
			return nil
		}
		id, m, err := findMail(p, args[1])
		if err != nil {
			return err
		}
		if method != "delete_mail" && m.expired(now) {
			return errors.New("邮件已过期")
		}
		switch method {
		case "read_mail":
			if m.State == MailUnread {
				if m.hasAttachments() {
					m.State = MailUnclaimed
				} else {
					m.State = MailFinal
				}
				m.ModifiedAt = now
			}
		case "receive_attachment":
			if err := claimMail(p, &m, now); err != nil {
				return err
			}
			changed = true
		case "delete_mail":
			if m.State != MailFinal && m.hasAttachments() && !m.expired(now) {
				return errors.New("邮件附件尚未领取")
			}
			delete(p.ShortMailInfo, id)
			return advanceMailRevision(p)
		default:
			return errors.New("邮件方法未实现")
		}
		p.ShortMailInfo[id] = m
		return advanceMailRevision(p)
	})
	if err != nil {
		var rejected *runeBusinessError
		if errors.As(err, &rejected) {
			code, ok := androidShop.Errors[rejected.Name]
			if !ok {
				return nil, errors.New("邮件容量错误码缺失")
			}
			return []Push{Callback(cb, []any{code})}, nil
		}
		return nil, err
	}
	pushes := []Push{push("Avatar", "client_prop_changed", []any{"short_mail_info", mailProperties(c.SelectedAvatarUnsafe().Progress.ShortMailInfo, now)})}
	if changed {
		pushes = append(pushes, materialManagerPush(c), cardMgrPush(c), runePush(c), knowledgePush(c), push("Avatar", "client_prop_changed", []any{"power", c.SelectedAvatarUnsafe().Progress.Power}))
		for key, value := range profileCosmeticProperties(c.SelectedAvatarUnsafe(), s.Now()) {
			pushes = append(pushes, push("Avatar", "client_prop_changed", []any{key, value}))
		}
		pushes = append(pushes, activityPushes(c, s.Now())...)
	}
	return append(pushes, Callback(cb, []any{RetSuccess})), nil
}

// SelectedAvatarUnsafe 仅在Service.Handle持有连接锁时使用。
func (c *Connection) SelectedAvatarUnsafe() Avatar {
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			return av
		}
	}
	return Avatar{}
}

func advanceMailRevision(p *Progress) error {
	if p.MailRevision < 0 || p.MailRevision == math.MaxInt64 {
		return errors.New("邮箱版本无效或溢出")
	}
	p.MailRevision++
	return nil
}

// 发件前检查所有未领取附件，防止两封邮件承诺相同资产编号。
func validateMailboxAssetIDs(p *Progress, m Mail) error {
	seen := map[string]bool{}
	for _, c := range p.Cards {
		seen[c.UUID] = true
	}
	for id := range p.Runes {
		seen[id] = true
	}
	for _, old := range p.ShortMailInfo {
		if old.State == MailFinal {
			continue
		}
		for _, c := range old.Cards {
			seen[c.UUID] = true
		}
		for id := range old.Runes {
			seen[id] = true
		}
	}
	for _, c := range m.Cards {
		if seen[c.UUID] {
			return errors.New("邮件幻书编号与库存或待领附件冲突")
		}
		seen[c.UUID] = true
	}
	for id := range m.Runes {
		if seen[id] {
			return errors.New("邮件契印编号与库存或待领附件冲突")
		}
		seen[id] = true
	}
	return nil
}
