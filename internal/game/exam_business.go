package game

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ExamTaskState struct {
	Count   int   `json:"finish_times"`
	Claimed []int `json:"bonus_times_list"`
}
type ExamDungeonState struct {
	Score   int   `json:"challenge_score"`
	Claimed []int `json:"bonus_score_list"`
}
type ExamState struct {
	Tasks          map[int]ExamTaskState    `json:"task_info"`
	Dungeons       map[int]ExamDungeonState `json:"dungeon_info"`
	Exams          map[int][]int            `json:"exam_2_bonus_score"`
	Study          []int                    `json:"study_dungeon_ids"`
	Answers        map[int]int              `json:"problem_ids"`
	Groups         []int                    `json:"group_ids"`
	Prev           map[int]int              `json:"prev_answers"`
	StudyRemaining int                      `json:"study_remaining"`
	Finished       int                      `json:"study_finished"`
	Storyline      map[string]string        `json:"storyline_bonus_dict"`
}

func examProperties(w *ExamState) map[string]any {
	return map[string]any{"task_info": w.Tasks, "dungeon_info": w.Dungeons, "exam_2_bonus_score": w.Exams, "study_dungeon_ids": w.Study, "problem_ids": w.Answers, "group_ids": w.Groups}
}

func ensureExam(p *Progress) *ExamState {
	if p.Activities.Exam == nil {
		p.Activities.Exam = &ExamState{Tasks: map[int]ExamTaskState{}, Dungeons: map[int]ExamDungeonState{}, Exams: map[int][]int{}, Study: []int{}, Answers: map[int]int{}, Groups: []int{}, Prev: map[int]int{}}
	}
	if p.Activities.Exam.Storyline == nil {
		p.Activities.Exam.Storyline = map[string]string{}
	}
	return p.Activities.Exam
}
func prepareExamDungeon(p *Progress, bc *ActivityBattleContext, day, level int, now time.Time) error {
	r := activityData("exam_activity", bc.DungeonID)
	w := ensureExam(p)
	if len(r) == 0 {
		for key := range androidActivities["exam_plot_dungeon"] {
			id, _ := strconv.Atoi(key)
			plot := activityData("exam_plot_dungeon", id)
			if plot.integer("plot_dungeon_id") == bc.DungeonID {
				next, e := examNextSpecial(*p)
				if e != nil {
					return e
				}
				if plot.integer("plot_type") == 1 && next != id && !containsInt(p.ClearedDungeons, bc.DungeonID) {
					return errors.New("考试剧情不在当前日程")
				}
				bc.Place = "plot"
				bc.NodeID = id
				return nil
			}
		}
		return errors.New("考试副本课程或剧情配置不存在")
	}
	bc.Tasks = r.ids("fix_tasks")
	if containsInt(activityData("dungeons", bc.DungeonID).ids("dungeon_sub_types"), 1) {
		if len(w.Study) >= activityData("exam_settings", 1).integer("study_dungeon_count") {
			return errors.New("考试学习总次数已达到原生上限")
		}
		next, e := examNextSpecial(*p)
		if e != nil {
			return e
		}
		if next != 0 {
			return errors.New("考试当前剧情或问题组尚未完成")
		}
		bc.Place = "study"
	}
	return nil
}

// Android92EDA415以完成课程历史加插强制剧情及问题组；学习不是预选副本列表。
func examNextSpecial(p Progress) (int, error) {
	w := p.Activities.Exam
	if w == nil {
		return activityData("exam_settings", 1).integer("start_plot_id"), nil
	}
	var rows []struct {
		Key   []json.RawMessage `json:"键"`
		Value struct {
			ID    int    `json:"id"`
			Kind  string `json:"type"`
			Count int    `json:"finish_count"`
			Group int    `json:"group"`
		} `json:"值"`
	}
	if json.Unmarshal(androidActivities["exam_special_dungeon"]["键值条目"], &rows) != nil {
		return 0, errors.New("考试特殊日程目录无效")
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Value.Count != rows[j].Value.Count {
			return rows[i].Value.Count < rows[j].Value.Count
		}
		if rows[i].Value.Kind != rows[j].Value.Kind {
			return rows[i].Value.Kind < rows[j].Value.Kind
		}
		return rows[i].Value.ID < rows[j].Value.ID
	})
	for _, row := range rows {
		v := row.Value
		if v.Count > len(w.Study) {
			break
		}
		if v.Kind == "problem" {
			if !containsInt(w.Groups, v.Group) {
				return -v.Group, nil
			}
			continue
		}
		r := activityData("exam_plot_dungeon", v.ID)
		did := r.integer("plot_dungeon_id")
		if did <= 0 {
			return 0, errors.New("考试日程剧情映射不存在")
		}
		if !containsInt(p.ClearedDungeons, did) && !containsInt(p.ClearedDungeons, r.integer("branch_plot")) {
			return v.ID, nil
		}
	}
	return 0, nil
}

func validateActivityDungeonExtra(p *Progress, bc *ActivityBattleContext, extra map[string]any, level int) error {
	if bc == nil {
		return nil
	}
	switch bc.ActivityID {
	case 208:
		if bc.Place == "explore" {
			if err := validateMikuBattleExtra(p, bc, extra); err != nil {
				return err
			}
			if bc.Angry {
				r := activityData("miku_battle_events", bc.Resource)
				skills, camps, shown := r.ids("battle_skill_list"), r.ids("battle_skill_camp_list"), r.ids("show_battle_skill")
				levels := r.ids("battle_skill_level_info")
				if len(levels) == 0 {
					levels = []int{1, 0, 0}
				}
				if len(camps) == 0 {
					camps = []int{0}
				}
				if len(levels) != 3 {
					return errors.New("初音愤怒战场技等级配置无效")
				}
				for i, skill := range skills {
					index := i
					if index >= len(camps) {
						index = len(camps) - 1
					}
					show := 0
					if containsInt(shown, i+1) {
						show = 1
					}
					bc.NativeSkills = append(bc.NativeSkills, [6]int{skill, camps[index], levels[0], levels[1], levels[2], show})
				}
			}
		}
	case 209:
		if bc.Place == "plot" {
			if v, ok := extra["task_ids"]; ok && v != nil {
				return errors.New("考试剧情不得注入学习任务")
			}
			return nil
		}
		r := activityData("exam_activity", bc.DungeonID)
		raw, err := json.Marshal(extra["task_ids"])
		if err != nil {
			return err
		}
		var selected []int
		if string(raw) != "null" && json.Unmarshal(raw, &selected) != nil {
			return errors.New("考试任务列表无效")
		}
		if len(r.ids("fix_tasks")) > 0 {
			if len(selected) > 0 {
				return errors.New("摸底考试不得替换固定任务")
			}
			return nil
		}
		want := r.integer("choose_task_count")
		var extraChoices [][]json.RawMessage
		if len(r["unlock_choose_task"]) > 0 && json.Unmarshal(r["unlock_choose_task"], &extraChoices) != nil {
			return errors.New("考试扩展任务配置无效")
		}
		for _, choice := range extraChoices {
			if len(choice) != 3 {
				return errors.New("考试扩展任务结构无效")
			}
			var count int
			var cond [][]json.RawMessage
			if json.Unmarshal(choice[0], &count) != nil || json.Unmarshal(choice[1], &cond) != nil || count < 0 {
				return errors.New("考试扩展任务参数无效")
			}
			ok, e := shopConditions([][][]json.RawMessage{cond}, *p, level)
			if e != nil {
				return e
			}
			if ok {
				want += count
			}
		}
		if len(selected) != want || want < 0 || want > 32 {
			return errors.New("考试选课任务数量无效")
		}
		mutex := map[int]bool{}
		seen := map[int]bool{}
		for _, id := range selected {
			t := activityData("exam_task", id)
			if len(t) == 0 || seen[id] || !containsInt(r.ids("choose_task_group"), t.integer("task_group")) {
				return errors.New("考试任务不在当前课程允许组")
			}
			if sub := t.integer("exclusive_subject"); sub > 0 && sub != r.integer("subject_id") {
				return errors.New("考试任务科目不匹配")
			}
			if m := t.integer("mutex_group"); m > 0 {
				if mutex[m] {
					return errors.New("考试任务互斥组冲突")
				}
				mutex[m] = true
			}
			if err := t.conditions("unlock_condition", *p, level); err != nil {
				return err
			}
			seen[id] = true
		}
		bc.Tasks = append([]int{}, selected...)
	case 211:
		w := p.Activities.Summer
		if w == nil {
			return errors.New("夏日状态丢失")
		}
		if len(w.Items) > 0 {
			v, ok := extra["item_id"]
			if !ok {
				return errors.New("夏日战斗必须选择已拥有道具")
			}
			raw, err := json.Marshal(v)
			if err != nil {
				return err
			}
			var item int
			if json.Unmarshal(raw, &item) != nil || !containsInt(w.Items, item) {
				return errors.New("夏日战斗道具未拥有")
			}
			bc.Resource = item
			w.LastItem = item
			r := activityData("summer_item", item)
			if r.flag("_disable") {
				return errors.New("夏日宝藏已禁用")
			}
			if skill := r.integer("skill_id"); skill > 0 {
				bc.NativeSkills = append(bc.NativeSkills, [6]int{skill, 1, 1, 0, 1, 1})
			}
		}
	}
	return nil
}
func finishExamDungeon(p *Progress, bc *ActivityBattleContext, tasks []int, now time.Time) error {
	w := p.Activities.Exam
	if w == nil {
		return errors.New("考试存档丢失")
	}
	if bc.Place == "plot" {
		return nil
	}
	score := 0
	for _, id := range tasks {
		t := activityData("exam_task", id)
		if len(t) == 0 || !containsInt(bc.Tasks, id) {
			return errors.New("考试结果任务没有注入")
		}
		amount := t.integer("task_score")
		if amount < 0 || score > math.MaxInt32-amount {
			return errors.New("考试成绩溢出")
		}
		score += amount
		v := w.Tasks[id]
		if v.Count == math.MaxInt32 {
			return errors.New("考试学习次数溢出")
		}
		v.Count++
		if v.Claimed == nil {
			v.Claimed = []int{}
		}
		w.Tasks[id] = v
	}
	d := w.Dungeons[bc.DungeonID]
	if score > d.Score {
		d.Score = score
	}
	if d.Claimed == nil {
		d.Claimed = []int{}
	}
	w.Dungeons[bc.DungeonID] = d
	if bc.Place == "study" {
		w.Study = append(w.Study, bc.DungeonID)
		w.StudyRemaining = activityData("exam_settings", 1).integer("study_dungeon_count") - len(w.Study)
		w.Finished = len(w.Study)
	}
	return nil
}
func claimExamStudy(p *Progress, id, times int, all bool, now time.Time) (map[string]any, error) {
	w := ensureExam(p)
	keys := []int{id}
	if all {
		keys = []int{}
		for k := range w.Tasks {
			keys = append(keys, k)
		}
		sort.Ints(keys)
	}
	bonuses := []int{}
	updates := map[int]ExamTaskState{}
	for _, key := range keys {
		v, ok := w.Tasks[key]
		if !ok {
			if all {
				continue
			}
			return nil, runeReject("RET_EXAM_STUDY_TIMES_NOT_REACH", "考试学习次数尚未达到")
		}
		var tiers [][]int
		if json.Unmarshal(activityData("exam_task", key)["exam_task_bonus"], &tiers) != nil {
			return nil, errors.New("考试任务奖励配置无效")
		}
		for _, tier := range tiers {
			if len(tier) != 2 {
				return nil, errors.New("考试任务奖励结构无效")
			}
			if v.Count >= tier[0] && !containsInt(v.Claimed, tier[0]) && (all || times <= 0 || times == tier[0]) {
				bonuses = append(bonuses, tier[1])
				v.Claimed = append(v.Claimed, tier[0])
			}
		}
		updates[key] = v
	}
	if len(bonuses) == 0 {
		return nil, errors.New("考试学习奖励没有可领取项")
	}
	box, err := grantActivityBonus(p, bonuses, now)
	if err != nil {
		return nil, err
	}
	for key, v := range updates {
		w.Tasks[key] = v
	}
	return box, nil
}
func claimExamScore(p *Progress, id, threshold int, test bool, now time.Time) (map[string]any, error) {
	w := ensureExam(p)
	var tiers [][]int
	claimed := []int{}
	best := 0
	d := w.Dungeons[id]
	if test {
		if json.Unmarshal(activityData("exam_test_bonus", id)["challenge_bonus"], &tiers) != nil {
			return nil, errors.New("摸底考试奖励配置无效")
		}
		ids := []int{}
		if json.Unmarshal(androidActivities["exam_times_2_dungeon"][strconv.Itoa(id)], &ids) != nil || len(ids) == 0 {
			return nil, errors.New("摸底考试科目映射不存在")
		}
		for _, did := range ids {
			best += w.Dungeons[did].Score
		}
		claimed = w.Exams[id]
	} else {
		if json.Unmarshal(activityData("exam_activity", id)["challenge_bonus"], &tiers) != nil {
			return nil, errors.New("复习考试奖励配置无效")
		}
		best = d.Score
		claimed = d.Claimed
	}
	bonuses := []int{}
	for _, tier := range tiers {
		if len(tier) != 2 {
			return nil, errors.New("考试成绩奖励结构无效")
		}
		if tier[0] <= best && !containsInt(claimed, tier[0]) && (threshold <= 0 || threshold == tier[0]) {
			bonuses = append(bonuses, tier[1])
			claimed = append(claimed, tier[0])
		}
	}
	if len(bonuses) == 0 {
		return nil, errors.New("考试成绩奖励未达标或已领取")
	}
	box, err := grantActivityBonus(p, bonuses, now)
	if err != nil {
		return nil, err
	}
	if test {
		w.Exams[id] = claimed
	} else {
		d.Claimed = claimed
		w.Dungeons[id] = d
	}
	return box, nil
}
func (s *Service) examRPC(ctx context.Context, c *Connection, method string, args []json.RawMessage) ([]Push, error) {
	direct := method == "prev_exam_choose_opt"
	lens := map[string]int{"prev_exam_choose_opt": 2, "receive_exam_study_bonus_all": 1, "receive_exam_study_bonus": 3, "receive_exam_review_bonus": 3, "receive_exam_test_bonus": 3, "exam_answer_problem": 2, "exam_answer_bonus": 2, "receive_storyline_bonus": 4}
	if len(args) != lens[method] {
		return nil, errors.New("考试接口参数数量无效")
	}
	var cb, id, value int
	if !direct {
		if json.Unmarshal(args[0], &cb) != nil || cb <= 0 {
			return nil, errors.New("考试回调编号无效")
		}
	}
	offset := 1
	if direct {
		offset = 0
	}
	if method != "exam_answer_problem" && method != "receive_storyline_bonus" && len(args) > offset {
		if json.Unmarshal(args[offset], &id) != nil {
			return nil, errors.New("考试编号无效")
		}
	}
	if len(args) > offset+1 && method != "receive_storyline_bonus" {
		if json.Unmarshal(args[offset+1], &value) != nil {
			return nil, errors.New("考试次数或分数无效")
		}
	}
	box := emptyActivityBox()
	err := s.updateProgress(ctx, c, func(p *Progress) error {
		level := c.SelectedAvatarUnsafe().Info.Level
		if _, err := activityOpen(*p, 209, level, s.Now()); err != nil {
			return err
		}
		w := ensureExam(p)
		var e error
		switch method {
		case "prev_exam_choose_opt":
			r := activityData("exam_prev_problem", id)
			if len(r) == 0 || value < 1 || value > 4 {
				return errors.New("摸底题目或选项无效")
			}
			if _, ok := w.Prev[id]; ok {
				return errors.New("摸底题目已经回答")
			}
			w.Prev[id] = value
			return nil
		case "receive_exam_study_bonus_all":
			box, e = claimExamStudy(p, 0, 0, true, s.Now())
			return e
		case "receive_exam_study_bonus":
			box, e = claimExamStudy(p, id, value, false, s.Now())
			return e
		case "receive_exam_review_bonus":
			box, e = claimExamScore(p, id, value, false, s.Now())
			return e
		case "receive_exam_test_bonus":
			box, e = claimExamScore(p, id, value, true, s.Now())
			return e
		case "exam_answer_problem":
			var answers map[int]int
			if json.Unmarshal(args[1], &answers) != nil || len(answers) == 0 || len(answers) > len(androidActivities["exam_problem"]) {
				return errors.New("考试答案字典无效")
			}
			next, e := examNextSpecial(*p)
			if e != nil {
				return e
			}
			if next >= 0 {
				return errors.New("当前日程没有待回答问题组")
			}
			for qid, answer := range answers {
				r := activityData("exam_problem", qid)
				if len(r) == 0 || r.integer("group") != -next || containsInt(w.Groups, -next) || answer < 1 || answer > len(r.ids("problem_option")) {
					var opts []string
					_ = json.Unmarshal(r["problem_option"], &opts)
					if len(r) == 0 || r.integer("group") != -next || containsInt(w.Groups, -next) || answer < 1 || answer > len(opts) {
						return errors.New("考试题目不属于当前问题组或选项无效")
					}
				}
				if e = r.conditions("unlock_condition", *p, level); e != nil {
					return e
				}
			}
			for qid, answer := range answers {
				w.Answers[qid] = answer
			}
			return nil
		case "exam_answer_bonus":
			var ids []int
			if json.Unmarshal(androidActivities["exam_problem_group_2_id"][strconv.Itoa(id)], &ids) != nil || len(ids) == 0 || containsInt(w.Groups, id) {
				return errors.New("考试问题组不存在或已领奖")
			}
			bonuses := []int{}
			for _, qid := range ids {
				r := activityData("exam_problem", qid)
				a, ok := w.Answers[qid]
				if !ok {
					return errors.New("考试问题组尚未全部回答")
				}
				if a == r.integer("correct_id") {
					bonuses = append(bonuses, r.integer("problem_bonus_id"))
				}
			}
			box, e = grantActivityBonus(p, bonuses, s.Now(), level)
			if e != nil {
				return e
			}
			w.Groups = append(w.Groups, id)
			return nil
		case "receive_storyline_bonus":
			var filename, node, option string
			if json.Unmarshal(args[1], &filename) != nil || json.Unmarshal(args[2], &node) != nil || json.Unmarshal(args[3], &option) != nil {
				return errors.New("考试剧情奖励参数无效")
			}
			var rows []struct {
				Value struct {
					File     string              `json:"storyline_filename"`
					Node     string              `json:"storyline_node_name"`
					Activity int                 `json:"game_activity_id"`
					Options  [][]json.RawMessage `json:"storyline_option_bonus"`
				} `json:"值"`
			}
			if json.Unmarshal(androidActivities["storyline_bonus"]["键值条目"], &rows) != nil {
				return errors.New("剧情选项奖励目录无效")
			}
			bonus := 0
			for _, row := range rows {
				r := row.Value
				if r.File != filename || r.Node != node || r.Activity != 209 {
					continue
				}
				for _, pair := range r.Options {
					if len(pair) != 2 {
						return errors.New("剧情奖励选项结构无效")
					}
					var name string
					var bid int
					if json.Unmarshal(pair[0], &name) != nil || json.Unmarshal(pair[1], &bid) != nil {
						return errors.New("剧情奖励选项无效")
					}
					if name == option {
						bonus = bid
					}
				}
			}
			key := strings.ReplaceAll(filename+"_"+node, ".", "")
			if bonus <= 0 || w.Storyline[key] != "" {
				return errors.New("剧情奖励选项不存在或已领取")
			}
			box, e = grantActivityBonus(p, []int{bonus}, s.Now(), level)
			if e != nil {
				return e
			}
			w.Storyline[key] = option
			return nil
		}
		return errors.New("考试业务不存在")
	})
	if err != nil {
		if direct {
			return nil, err
		}
		if method == "exam_answer_problem" {
			return []Push{Callback(cb, []any{activityErrorCode(err)})}, nil
		}
		return []Push{Callback(cb, []any{activityErrorCode(err), emptyActivityBox()})}, nil
	}
	out := activityPushes(c, s.Now())
	if direct {
		return out, nil
	}
	if method == "exam_answer_problem" {
		return append(out, Callback(cb, []any{RetSuccess})), nil
	}
	return append(out, materialManagerPush(c), knowledgePush(c), Callback(cb, []any{RetSuccess, box})), nil
}
