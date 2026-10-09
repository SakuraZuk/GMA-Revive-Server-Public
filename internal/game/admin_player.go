package game

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// 管理查询只暴露角色与游戏状态，不返回账号口令或重连凭据。
type AdminAccounts interface {
	AdminPlayers(context.Context, string, int) ([]Avatar, error)
	AdminPlayer(context.Context, []byte) (Avatar, error)
	AdminUpdatePlayer(context.Context, []byte, func(*Avatar) error) (Avatar, error)
}

type PlayerAdminReceipt struct {
	Fingerprint string              `json:"fingerprint"`
	Operator    string              `json:"operator"`
	Reason      string              `json:"reason"`
	Time        int64               `json:"time"`
	Operation   string              `json:"operation"`
	Result      map[string]any      `json:"result"`
	Request     *playerAdminRequest `json:"request,omitempty"`
}

type playerAdminRequest struct {
	AvatarOID  string              `json:"avatar_oid"`
	Receipt    string              `json:"receipt"`
	Operator   string              `json:"operator"`
	Reason     string              `json:"reason"`
	Operation  string              `json:"operation"`
	Nickname   string              `json:"nickname,omitempty"`
	Level      int                 `json:"level,omitempty"`
	Gender     int                 `json:"gender,omitempty"`
	MaterialID int                 `json:"material_id,omitempty"`
	Amount     int64               `json:"amount,omitempty"`
	CardID     int                 `json:"card_id,omitempty"`
	Count      int                 `json:"count,omitempty"`
	Offer      *ShopRecommendation `json:"offer,omitempty"`
}

func adminPlayerView(av Avatar, detail bool) map[string]any {
	out := map[string]any{"avatar_oid": hex.EncodeToString(av.OID), "uid": av.UID, "hostnum": av.Hostnum, "nickname": av.Info.Nickname, "level": av.Info.Level, "gender": av.Gender}
	if detail {
		out["progress"] = av.Progress
		out["资料"] = av.Info
		out["战斗记录说明"] = "当前战斗最多保留128条观察事件；不是全量回放"
	}
	return out
}

func registerPlayerAdmin(mux *http.ServeMux, s *Service) {
	writeJSON := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("GET /admin/players", func(w http.ResponseWriter, r *http.Request) {
		store, ok := s.Accounts.(AdminAccounts)
		if !ok {
			http.Error(w, "存储不支持玩家管理", 503)
			return
		}
		q := r.URL.Query().Get("q")
		if len(q) > 100 {
			http.Error(w, "查询条件过长", 400)
			return
		}
		list, err := store.AdminPlayers(r.Context(), q, 100)
		if err != nil {
			log.Printf("管理玩家查询失败：%v", err)
			http.Error(w, "玩家查询失败", 500)
			return
		}
		rows := []map[string]any{}
		for _, av := range list {
			rows = append(rows, adminPlayerView(av, false))
		}
		writeJSON(w, map[string]any{"players": rows, "上限": 100})
	})
	mux.HandleFunc("GET /admin/players/{oid}", func(w http.ResponseWriter, r *http.Request) {
		store, ok := s.Accounts.(AdminAccounts)
		if !ok {
			http.Error(w, "存储不支持玩家管理", 503)
			return
		}
		id := r.PathValue("oid")
		if !validObjectID(id) {
			http.Error(w, "角色编号无效", 400)
			return
		}
		oid, _ := hex.DecodeString(id)
		av, err := store.AdminPlayer(r.Context(), oid)
		if err != nil {
			http.Error(w, "角色读取失败或不存在", 404)
			return
		}
		writeJSON(w, adminPlayerView(av, true))
	})
	mux.HandleFunc("POST /admin/player/update", func(w http.ResponseWriter, r *http.Request) {
		var req playerAdminRequest
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF || !validObjectID(req.AvatarOID) || !adminAuditText(req.Receipt, 100) || !adminAuditText(req.Operator, 64) || !adminAuditText(req.Reason, 512) {
			http.Error(w, "请求或审计字段无效", 400)
			return
		}
		store, ok := s.Accounts.(AdminAccounts)
		if !ok {
			http.Error(w, "存储不支持玩家管理", 503)
			return
		}
		raw, _ := json.Marshal(req)
		sum := sha256.Sum256(raw)
		fingerprint := hex.EncodeToString(sum[:])
		oid, _ := hex.DecodeString(req.AvatarOID)
		var receipt PlayerAdminReceipt
		replayed := false
		var rejection error
		av, err := store.AdminUpdatePlayer(r.Context(), oid, func(av *Avatar) error {
			if old, exists := av.Progress.PlayerAdminReceipts[req.Receipt]; exists {
				if old.Fingerprint != fingerprint {
					rejection = errors.New("同一收据内容冲突")
					return rejection
				}
				receipt = old
				replayed = true
				return nil
			}
			if len(av.Progress.PlayerAdminReceipts) >= 10000 {
				rejection = errors.New("管理收据容量已满，需归档")
				return rejection
			}
			if rejection = applyPlayerAdmin(av, req, s); rejection != nil {
				return rejection
			}
			reconcileAchievementState(&av.Progress, av.Info.Level, s.Now())
			receipt = PlayerAdminReceipt{Fingerprint: fingerprint, Operator: req.Operator, Reason: req.Reason, Time: s.Now().Unix(), Operation: req.Operation, Result: adminPlayerView(*av, false), Request: &req}
			if av.Progress.PlayerAdminReceipts == nil {
				av.Progress.PlayerAdminReceipts = map[string]PlayerAdminReceipt{}
			}
			av.Progress.PlayerAdminReceipts[req.Receipt] = receipt
			return nil
		})
		if err != nil {
			if rejection != nil {
				http.Error(w, rejection.Error(), 422)
			} else {
				log.Printf("玩家管理事务失败：%v", err)
				http.Error(w, "管理事务失败，未修改", 500)
			}
			return
		}
		if !replayed {
			s.publishPlayerRefresh(av.OID)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if !replayed {
			w.WriteHeader(201)
		}
		writeJSON(w, map[string]any{"状态": "成功", "重复请求": replayed, "审计": receipt, "同步方式": "在线连接下一次Tick读取最新存档；离线玩家重登录"})
	})
}

func applyPlayerAdmin(av *Avatar, r playerAdminRequest, s *Service) error {
	p := &av.Progress
	if r.Operation != "recommendation_issue" && r.Offer != nil {
		return errors.New("推荐报价与其他操作冲突")
	}
	// 进行中的战斗冻结资产快照；管理修改必须等待该战斗结束。
	if p.Battle != nil && !p.Battle.Finished && p.Battle.Status != "finished" {
		return errors.New("角色正在战斗，请结束后再修改")
	}
	switch r.Operation {
	case "recommendation_issue":
		if r.Offer == nil || r.Nickname != "" || r.Level != 0 || r.Gender != 0 || r.MaterialID != 0 || r.Amount != 0 || r.CardID != 0 || r.Count != 0 {
			return errors.New("推荐发行字段无效")
		}
		return issueShopRecommendation(p, *r.Offer, s.Now())
	case "profile":
		if r.MaterialID != 0 || r.Amount != 0 || r.CardID != 0 || r.Count != 0 {
			return errors.New("资料操作不能带资产字段")
		}
		if r.Nickname == "" && r.Level == 0 && r.Gender == 0 {
			return errors.New("未指定资料修改")
		}
		if r.Nickname != "" {
			if ValidateNickname(r.Nickname) != 0 {
				return errors.New("昵称不符合原生规则")
			}
			av.Info.Nickname = r.Nickname
			av.NicknameSet = true
		}
		if r.Level != 0 {
			if _, ok := clientBaseline.LevelPower[r.Level]; !ok || r.Level > androidAvatarGrowth.Limits.Max {
				return errors.New("馆主等级不在Android目录内")
			}
			av.Info.Level = r.Level
			p.AvatarLevel = r.Level
			p.AvatarExp = 0
			p.Power.Max = clientBaseline.LevelPower[r.Level].Max
		}
		if r.Gender != 0 {
			if r.Gender != 1 && r.Gender != 2 {
				return errors.New("馆主性别无效")
			}
			av.Gender = r.Gender
		}
	case "material_add", "material_set":
		if r.Nickname != "" || r.Level != 0 || r.Gender != 0 || r.CardID != 0 || r.Count != 0 {
			return errors.New("材料操作字段冲突")
		}
		mat, ok := androidShop.Materials[r.MaterialID]
		if !ok || (mat.Type != 3 && mat.Type != 4) || r.Amount < 0 {
			return errors.New("仅可修改Android普通库存材料，数量须非负")
		}
		m := p.Materials[r.MaterialID]
		if m.Count < 0 || m.Total < 0 {
			return errors.New("材料存档无效")
		}
		var increment int64
		if r.Operation == "material_add" {
			increment = r.Amount
			if m.Count > math.MaxInt64-increment {
				return errors.New("材料余额溢出")
			}
			m.Count += increment
		} else {
			if r.Amount > m.Count {
				increment = r.Amount - m.Count
			}
			m.Count = r.Amount
		}
		if m.Total > math.MaxInt64-increment || (mat.Limit > 0 && m.Count > mat.Limit) {
			return errors.New("材料累计值或容量溢出")
		}
		m.Total += increment
		m.ID = r.MaterialID
		if p.Materials == nil {
			p.Materials = map[int]Material{}
		}
		p.Materials[r.MaterialID] = m
	case "card_grant":
		if r.Nickname != "" || r.Level != 0 || r.Gender != 0 || r.MaterialID != 0 || r.Amount != 0 {
			return errors.New("幻书操作字段冲突")
		}
		rules, err := loadIntimacyCatalog()
		if err != nil {
			return err
		}
		card, ok := rules.Cards[r.CardID]
		if !ok || card.Disabled != 0 || r.Count < 1 || r.Count > 50 || len(p.Cards)+r.Count > 2000 {
			return errors.New("幻书模板、数量或容量无效")
		}
		for i := 0; i < r.Count; i++ {
			appendOwnedCard(p, newCard(r.CardID, 1, s.Now()))
		}
	default:
		return errors.New("未知管理操作")
	}
	return nil
}

func (a *FixtureAccounts) AdminPlayers(ctx context.Context, q string, limit int) ([]Avatar, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := []Avatar{}
	for _, r := range a.records {
		for _, av := range r.Avatars {
			if q == "" || strings.Contains(av.Info.Nickname, q) || strconv.FormatInt(av.UID, 10) == q || hex.EncodeToString(av.OID) == q {
				av.Progress = Progress{}
				av.Account = ""
				av.OID = append([]byte(nil), av.OID...)
				out = append(out, av)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (a *FixtureAccounts) AdminPlayer(ctx context.Context, oid []byte) (Avatar, error) {
	return a.adminFixturePlayer(ctx, oid, nil)
}
func (a *FixtureAccounts) AdminUpdatePlayer(ctx context.Context, oid []byte, f func(*Avatar) error) (Avatar, error) {
	return a.adminFixturePlayer(ctx, oid, f)
}
func (a *FixtureAccounts) adminFixturePlayer(ctx context.Context, oid []byte, f func(*Avatar) error) (Avatar, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Avatar{}, err
	}
	for key, r := range a.records {
		for i, av := range r.Avatars {
			if string(av.OID) != string(oid) {
				continue
			}
			av.Progress = CloneProgress(av.Progress)
			av.OID = append([]byte(nil), av.OID...)
			if f != nil {
				if err := f(&av); err != nil {
					return Avatar{}, err
				}
				for _, record := range a.records {
					for _, other := range record.Avatars {
						if string(other.OID) != string(oid) && other.Hostnum == av.Hostnum && other.NicknameSet && av.NicknameSet && other.Info.Nickname == av.Info.Nickname {
							return Avatar{}, ErrNicknameExists
						}
					}
				}
				r.Avatars[i] = av
				a.records[key] = r
			}
			av.Progress = CloneProgress(av.Progress)
			return av, nil
		}
	}
	return Avatar{}, errors.New("角色不存在")
}
