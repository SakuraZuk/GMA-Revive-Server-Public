package game

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAdminUser     = "admin"
	defaultAdminPassword = "qwerasdf1234"
	adminSessionCookie   = "hs_ops"
	adminSessionTTL      = 12 * time.Hour
)

// OpsConfigAccounts 保存运营公告、全服邮件和卡池排期。没有该接口时只保存在本进程内存。
type OpsConfigAccounts interface {
	LoadOpsConfig(context.Context) (map[string]json.RawMessage, error)
	SaveOpsConfig(context.Context, string, json.RawMessage) error
}

type adminSession struct {
	User    string
	Expires time.Time
}

type opsNotice struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Kind      string `json:"kind"`
	Pinned    bool   `json:"pinned"`
	Enabled   bool   `json:"enabled"`
	Begin     int64  `json:"begin"`
	End       int64  `json:"end"`
	Operator  string `json:"operator"`
	UpdatedAt int64  `json:"updated_at"`
}

type opsBroadcast struct {
	Receipt   string `json:"receipt"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Sender    string `json:"sender"`
	Material  int    `json:"material_id,omitempty"`
	Amount    int64  `json:"amount,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Operator  string `json:"operator"`
	Reason    string `json:"reason"`
	Sent      int    `json:"sent"`
	Time      int64  `json:"time"`
}

type opsGachaWindow struct {
	PoolID   int    `json:"pool_id"`
	Enabled  bool   `json:"enabled"`
	Begin    int64  `json:"begin"`
	End      int64  `json:"end"`
	Note     string `json:"note,omitempty"`
	Operator string `json:"operator"`
	Updated  int64  `json:"updated_at"`
}

type opsState struct {
	mu         sync.Mutex
	loaded     bool
	secret     []byte
	sessions   map[string]adminSession
	notices    []opsNotice
	broadcasts []opsBroadcast
	gacha      map[int]opsGachaWindow
	loginFails map[string][]time.Time
}

func (s *Service) ops() *opsState {
	value, _ := opsStates.LoadOrStore(s, &opsState{sessions: map[string]adminSession{}, gacha: map[int]opsGachaWindow{}, loginFails: map[string][]time.Time{}})
	state := value.(*opsState)
	if state.secret == nil {
		state.mu.Lock()
		if state.secret == nil {
			secret := make([]byte, 32)
			if _, err := rand.Read(secret); err != nil {
				secret = sha256.New().Sum([]byte(strconv.FormatInt(time.Now().UnixNano(), 10)))
			}
			state.secret = secret
		}
		state.mu.Unlock()
	}
	return state
}

var opsStates sync.Map

func registerOpsAdmin(mux *http.ServeMux, s *Service) {
	writeJSON := func(w http.ResponseWriter, code int, value any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("POST /admin/session", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			User     string `json:"user"`
			Password string `json:"password"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
			http.Error(w, "登录请求无效", http.StatusBadRequest)
			return
		}
		user, pass := adminCredentials()
		source := adminClientHost(r)
		if s.ops().loginLimited(source, s.Now()) {
			http.Error(w, "登录失败过多，请稍后再试", http.StatusTooManyRequests)
			return
		}
		if subtle.ConstantTimeCompare([]byte(req.User), []byte(user)) != 1 || subtle.ConstantTimeCompare([]byte(req.Password), []byte(pass)) != 1 {
			s.ops().noteLoginFailure(source, s.Now())
			http.Error(w, "管理员账号或密码不正确", http.StatusUnauthorized)
			return
		}
		s.ops().clearLoginFailures(source)
		token, expires, err := s.ops().issueSession(user, s.Now())
		if err != nil {
			http.Error(w, "登录会话创建失败", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: adminSessionCookie, Value: token, Path: "/admin", HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: expires})
		writeJSON(w, http.StatusOK, map[string]any{"状态": "已登录", "user": user, "expires": expires.Unix()})
	})
	mux.HandleFunc("POST /admin/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(adminSessionCookie); err == nil {
			s.ops().dropSession(c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: adminSessionCookie, Value: "", Path: "/admin", HttpOnly: true, MaxAge: -1})
		writeJSON(w, http.StatusOK, map[string]any{"状态": "已退出"})
	})
	mux.HandleFunc("GET /admin/session", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.adminUser(r)
		if !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": user})
	})
	mux.HandleFunc("GET /admin/overview", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		online := s.onlineSnapshot()
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		activities := activityAdminRows(s.Now())
		open := 0
		for _, row := range activities {
			if row["状态"] == "进行中" {
				open++
			}
		}
		state.mu.Lock()
		noticeCount := len(state.notices)
		mailCount := len(state.broadcasts)
		poolCount := len(state.gacha)
		state.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"在线人数": len(online), "活动进行中": open, "活动总数": len(activities),
			"公告": noticeCount, "全服邮件": mailCount, "卡池排期": poolCount,
			"服务器时间": s.Now().Unix(),
		})
	})
	mux.HandleFunc("GET /admin/online", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		rows := s.onlineSnapshot()
		writeJSON(w, http.StatusOK, map[string]any{"count": len(rows), "players": rows, "说明": "在线人数来自当前进程的游戏连接；同一角色多端会计入多条连接。"})
	})
	mux.HandleFunc("GET /admin/activities", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"activities": activityAdminRows(s.Now()), "说明": "展示当前已加载的运营活动日程；改期仍写入活动日程文件后重载进程。"})
	})
	mux.HandleFunc("GET /admin/notices", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		state.mu.Lock()
		rows := append([]opsNotice(nil), state.notices...)
		state.mu.Unlock()
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Pinned != rows[j].Pinned {
				return rows[i].Pinned
			}
			return rows[i].UpdatedAt > rows[j].UpdatedAt
		})
		writeJSON(w, http.StatusOK, map[string]any{"notices": rows})
	})
	mux.HandleFunc("POST /admin/notices", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.adminUser(r)
		if !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		var req opsNotice
		if !decodeAdmin(w, r, &req) || !adminAuditText(req.Title, 80) || !adminAuditText(req.Body, 4000) {
			http.Error(w, "公告标题或正文无效", http.StatusBadRequest)
			return
		}
		if req.Kind == "" {
			req.Kind = "维护"
		}
		if req.Kind != "维护" && req.Kind != "活动" && req.Kind != "补偿" && req.Kind != "系统" {
			http.Error(w, "公告类型无效", http.StatusBadRequest)
			return
		}
		if req.End != 0 && req.End <= req.Begin {
			http.Error(w, "公告展示时段无效", http.StatusBadRequest)
			return
		}
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		req.ID = newBattleUUID(s.Now())
		req.Operator = user
		req.UpdatedAt = s.Now().Unix()
		state.mu.Lock()
		if len(state.notices) >= 200 {
			state.mu.Unlock()
			http.Error(w, "公告已达200条，请先删除旧公告", http.StatusUnprocessableEntity)
			return
		}
		state.notices = append(state.notices, req)
		raw, _ := json.Marshal(state.notices)
		state.mu.Unlock()
		if err := saveOps(r.Context(), s.Accounts, "notices", raw); err != nil {
			log.Printf("公告保存失败：%v", err)
			http.Error(w, "公告已写入内存，数据库保存失败", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"状态": "已发布", "notice": req})
	})
	mux.HandleFunc("POST /admin/notices/remove", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if !decodeAdmin(w, r, &req) || !validObjectID(req.ID) {
			http.Error(w, "公告编号无效", http.StatusBadRequest)
			return
		}
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		state.mu.Lock()
		next := state.notices[:0]
		found := false
		for _, n := range state.notices {
			if n.ID == req.ID {
				found = true
				continue
			}
			next = append(next, n)
		}
		state.notices = next
		raw, _ := json.Marshal(state.notices)
		state.mu.Unlock()
		if !found {
			http.Error(w, "公告不存在", http.StatusNotFound)
			return
		}
		if err := saveOps(r.Context(), s.Accounts, "notices", raw); err != nil {
			http.Error(w, "公告删除未持久化", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"状态": "已删除"})
	})
	mux.HandleFunc("GET /admin/broadcasts", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		state.mu.Lock()
		rows := append([]opsBroadcast(nil), state.broadcasts...)
		state.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"mails": rows})
	})
	mux.HandleFunc("POST /admin/broadcasts", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.adminUser(r)
		if !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		var req opsBroadcast
		if !decodeAdmin(w, r, &req) || !adminAuditText(req.Receipt, 100) || !adminAuditText(req.Title, 80) || !adminAuditText(req.Content, 4000) || !adminAuditText(req.Reason, 512) {
			http.Error(w, "全服邮件字段无效", http.StatusBadRequest)
			return
		}
		if req.Sender == "" {
			req.Sender = "阿克夏馆务处"
		}
		if !adminAuditText(req.Sender, 40) || (req.Amount != 0 && (req.Material <= 0 || req.Amount < 0)) {
			http.Error(w, "发件人或附件无效", http.StatusBadRequest)
			return
		}
		store, ok := s.Accounts.(SocialAccounts)
		if !ok {
			http.Error(w, "存储不支持全服邮件", http.StatusServiceUnavailable)
			return
		}
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		state.mu.Lock()
		for _, old := range state.broadcasts {
			if old.Receipt == req.Receipt {
				state.mu.Unlock()
				writeJSON(w, http.StatusOK, map[string]any{"状态": "成功", "重复请求": true, "sent": old.Sent})
				return
			}
		}
		state.mu.Unlock()
		mail := Mail{Title: req.Title, Content: req.Content, Sender: req.Sender, ExpiresAt: req.ExpiresAt}
		if req.Amount > 0 {
			mail.Attachments = map[int]int64{req.Material: req.Amount}
		}
		sent := 0
		rows, err := store.UpdateAllSocial(r.Context(), func(v map[string]*Avatar) error {
			ids := make([]string, 0, len(v))
			for id := range v {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				av := v[id]
				if _, exists := av.Progress.MailAdminReceipts[req.Receipt]; exists {
					continue
				}
				item := mail
				item.MID = nextMailMID(&av.Progress)
				if err := s.prepareAndInsertMail(&av.Progress, item); err != nil {
					return err
				}
				m := av.Progress.ShortMailInfo[item.MID]
				if av.Progress.MailAdminReceipts == nil {
					av.Progress.MailAdminReceipts = map[string]MailAdminReceipt{}
				}
				av.Progress.MailAdminReceipts[req.Receipt] = MailAdminReceipt{Operator: user, Reason: req.Reason, Time: s.Now().Unix(), MID: m.MID, UUID: m.UUID}
				sent++
			}
			return nil
		})
		if err != nil {
			log.Printf("全服邮件失败：%v", err)
			http.Error(w, "全服邮件未发送："+err.Error(), http.StatusUnprocessableEntity)
			return
		}
		for _, av := range rows {
			s.publishMailbox(av.OID, av.Progress)
		}
		req.Operator, req.Sent, req.Time = user, sent, s.Now().Unix()
		state.mu.Lock()
		state.broadcasts = append(state.broadcasts, req)
		if len(state.broadcasts) > 100 {
			state.broadcasts = state.broadcasts[len(state.broadcasts)-100:]
		}
		raw, _ := json.Marshal(state.broadcasts)
		state.mu.Unlock()
		_ = saveOps(r.Context(), s.Accounts, "broadcasts", raw)
		writeJSON(w, http.StatusCreated, map[string]any{"状态": "已发送", "sent": sent, "在线同步": len(rows)})
	})
	mux.HandleFunc("GET /admin/gacha", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pools": s.gachaAdminRows(r.Context())})
	})
	mux.HandleFunc("POST /admin/gacha", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.adminUser(r)
		if !ok {
			http.Error(w, "未登录", http.StatusUnauthorized)
			return
		}
		var req opsGachaWindow
		if !decodeAdmin(w, r, &req) {
			http.Error(w, "卡池排期无效", http.StatusBadRequest)
			return
		}
		if _, exists := androidGachaPools[req.PoolID]; !exists || req.Begin <= 0 || req.End <= req.Begin || req.End-req.Begin > 366*86400 || len(req.Note) > 200 {
			http.Error(w, "卡池或排期时段无效", http.StatusBadRequest)
			return
		}
		req.Operator, req.Updated = user, s.Now().Unix()
		state := s.ops()
		state.ensureLoaded(r.Context(), s.Accounts)
		state.mu.Lock()
		state.gacha[req.PoolID] = req
		raw, _ := json.Marshal(gachaList(state.gacha))
		state.mu.Unlock()
		if err := saveOps(r.Context(), s.Accounts, "gacha_schedules", raw); err != nil {
			http.Error(w, "卡池排期未持久化", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"状态": "已排期", "pool": req, "说明": "排期供运营查看；客户端抽卡仍按原生活动开放条件执行。"})
	})
}

func (s *Service) adminUser(r *http.Request) (string, bool) {
	if c, err := r.Cookie(adminSessionCookie); err == nil {
		if user, ok := s.ops().sessionUser(c.Value, s.Now()); ok {
			return user, true
		}
	}
	return "", false
}

func adminAuthorized(r *http.Request, token string, s *Service) bool {
	if len(token) >= 32 && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) == 1 {
		return true
	}
	_, ok := s.adminUser(r)
	return ok
}

func adminCredentials() (string, string) {
	user, pass := defaultAdminUser, defaultAdminPassword
	return user, pass
}

func (o *opsState) issueSession(user string, now time.Time) (string, time.Time, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	id := hex.EncodeToString(buf)
	expires := now.Add(adminSessionTTL)
	mac := hmac.New(sha256.New, o.secret)
	_, _ = mac.Write([]byte(id + "|" + user + "|" + strconv.FormatInt(expires.Unix(), 10)))
	token := base64.RawURLEncoding.EncodeToString([]byte(id + "|" + user + "|" + strconv.FormatInt(expires.Unix(), 10) + "|" + hex.EncodeToString(mac.Sum(nil))))
	o.mu.Lock()
	o.sessions[id] = adminSession{User: user, Expires: expires}
	o.mu.Unlock()
	return token, expires, nil
}

func (o *opsState) sessionUser(token string, now time.Time) (string, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", false
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || now.Unix() > exp {
		return "", false
	}
	mac := hmac.New(sha256.New, o.secret)
	_, _ = mac.Write([]byte(parts[0] + "|" + parts[1] + "|" + parts[2]))
	if subtle.ConstantTimeCompare([]byte(parts[3]), []byte(hex.EncodeToString(mac.Sum(nil)))) != 1 {
		return "", false
	}
	o.mu.Lock()
	session, ok := o.sessions[parts[0]]
	o.mu.Unlock()
	if !ok || session.User != parts[1] || now.After(session.Expires) {
		return "", false
	}
	return session.User, true
}

func adminClientHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (o *opsState) loginLimited(source string, now time.Time) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(recentLoginFails(o.loginFails[source], now)) >= 8
}

func (o *opsState) noteLoginFailure(source string, now time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.loginFails == nil {
		o.loginFails = map[string][]time.Time{}
	}
	o.loginFails[source] = append(recentLoginFails(o.loginFails[source], now), now)
}

func (o *opsState) clearLoginFailures(source string) {
	o.mu.Lock()
	delete(o.loginFails, source)
	o.mu.Unlock()
}

func recentLoginFails(times []time.Time, now time.Time) []time.Time {
	out := times[:0]
	for _, t := range times {
		if now.Sub(t) < 10*time.Minute {
			out = append(out, t)
		}
	}
	return out
}

func (o *opsState) dropSession(token string) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return
	}
	id := strings.SplitN(string(raw), "|", 2)[0]
	o.mu.Lock()
	delete(o.sessions, id)
	o.mu.Unlock()
}

func (o *opsState) ensureLoaded(ctx context.Context, accounts Accounts) {
	o.mu.Lock()
	if o.loaded {
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	store, ok := accounts.(OpsConfigAccounts)
	if !ok {
		o.mu.Lock()
		o.loaded = true
		o.mu.Unlock()
		return
	}
	raw, err := store.LoadOpsConfig(ctx)
	if err != nil {
		log.Printf("运营配置读取失败：%v", err)
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.loaded {
		return
	}
	_ = json.Unmarshal(raw["notices"], &o.notices)
	_ = json.Unmarshal(raw["broadcasts"], &o.broadcasts)
	var windows []opsGachaWindow
	_ = json.Unmarshal(raw["gacha_schedules"], &windows)
	if o.gacha == nil {
		o.gacha = map[int]opsGachaWindow{}
	}
	for _, w := range windows {
		o.gacha[w.PoolID] = w
	}
	o.loaded = true
}

func saveOps(ctx context.Context, accounts Accounts, kind string, raw []byte) error {
	store, ok := accounts.(OpsConfigAccounts)
	if !ok {
		return nil
	}
	return store.SaveOpsConfig(ctx, kind, raw)
}

func decodeAdmin(w http.ResponseWriter, r *http.Request, dest any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(dest) == nil && d.Decode(new(any)) == io.EOF
}

func nextMailMID(p *Progress) int {
	maxID := 0
	for id := range p.ShortMailInfo {
		if id > maxID {
			maxID = id
		}
	}
	if maxID < 100000 {
		return 100000
	}
	return maxID + 1
}

type onlineRow struct {
	AvatarOID string `json:"avatar_oid"`
	UID       int64  `json:"uid"`
	Hostnum   int    `json:"hostnum"`
	Nickname  string `json:"nickname"`
	Level     int    `json:"level"`
	Phase     string `json:"phase"`
	Device    string `json:"device"`
	Battle    bool   `json:"battle"`
}

func (s *Service) onlineSnapshot() []onlineRow {
	s.onlineMu.Lock()
	conns := []*Connection{}
	for _, set := range s.online {
		for c := range set {
			conns = append(conns, c)
		}
	}
	s.onlineMu.Unlock()
	rows := []onlineRow{}
	for _, c := range conns {
		c.mu.Lock()
		av, ok := avatarOf(c)
		row := onlineRow{Phase: string(c.phase), Device: maskDevice(c.deviceID), Battle: c.battleStartSent}
		if ok {
			row.AvatarOID = hex.EncodeToString(av.OID)
			row.UID = av.UID
			row.Hostnum = av.Hostnum
			row.Nickname = av.Info.Nickname
			row.Level = av.Info.Level
		}
		c.mu.Unlock()
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UID == rows[j].UID {
			return rows[i].Device < rows[j].Device
		}
		return rows[i].UID < rows[j].UID
	})
	return rows
}

func avatarOf(c *Connection) (Avatar, bool) {
	for _, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			return av, true
		}
	}
	return Avatar{}, false
}

func maskDevice(id string) string {
	if id == "" {
		return ""
	}
	if len(id) <= 6 {
		return "···"
	}
	return id[:3] + "···" + id[len(id)-3:]
}

func activityAdminRows(now time.Time) []map[string]any {
	ids := []int{}
	for key := range androidActivities["activity_type"] {
		id, _ := strconv.Atoi(key)
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	rows := []map[string]any{}
	for _, id := range ids {
		row := activityData("activity_type", id)
		w := activitySchedule(id)
		status := "未排期"
		if w.Begin > 0 {
			switch {
			case !w.Enabled:
				status = "已关闭"
			case now.Unix() < w.Begin:
				status = "未开始"
			case w.Permanent || now.Unix() <= w.End:
				status = "进行中"
			default:
				status = "已结束"
			}
		}
		rows = append(rows, map[string]any{
			"activity_id": id, "name": activityText(row, "name"), "enabled": w.Enabled, "permanent": w.Permanent,
			"begin": w.Begin, "end": w.End, "状态": status, "need_power": row.integer("need_power"),
		})
	}
	return rows
}

func activityText(row activityRow, key string) string {
	var text string
	_ = json.Unmarshal(row[key], &text)
	return text
}

func (s *Service) gachaAdminRows(ctx context.Context) []map[string]any {
	state := s.ops()
	state.ensureLoaded(ctx, s.Accounts)
	state.mu.Lock()
	windows := map[int]opsGachaWindow{}
	for id, w := range state.gacha {
		windows[id] = w
	}
	state.mu.Unlock()
	ids := make([]int, 0, len(androidGachaPools))
	for id := range androidGachaPools {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	now := s.Now().Unix()
	rows := []map[string]any{}
	for _, id := range ids {
		cfg := androidGachaPools[id]
		native := androidNativeDrawPools[id]
		w := windows[id]
		status := "常驻目录"
		if w.PoolID != 0 {
			switch {
			case !w.Enabled:
				status = "排期关闭"
			case now < w.Begin:
				status = "即将开放"
			case now <= w.End:
				status = "排期中"
			default:
				status = "排期结束"
			}
		}
		rows = append(rows, map[string]any{
			"pool_id": id, "cards": len(cfg.Cards), "cost_material": cfg.CostMaterial, "cost_count": cfg.CostCount,
			"limit": cfg.LimitCount, "guarantee": cfg.GuaranteeRarity, "activity": native.Activity,
			"schedule": w, "状态": status,
		})
	}
	return rows
}

func gachaList(in map[int]opsGachaWindow) []opsGachaWindow {
	out := make([]opsGachaWindow, 0, len(in))
	for _, w := range in {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PoolID < out[j].PoolID })
	return out
}

var errOpsKind = errors.New("运营配置类型无效")
