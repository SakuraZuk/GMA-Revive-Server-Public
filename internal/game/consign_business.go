package game

import "time"

type ConsignTaskState struct {
	FinishTime int64 `json:"finish_time"`
	StartTime  int64 `json:"start_time"`
	Cards      []int `json:"cards"`
	Index      int   `json:"index"`
	Level      int   `json:"level"`
}
type ConsignState struct {
	Tasks           map[int]*ConsignTaskState `json:"tasks"`
	FinishTimes     int                       `json:"finish_times"`
	RefreshTimes    int                       `json:"refresh_times"`
	NextRefresh     int64                     `json:"next_refresh"`
	RefreshPoint    int64                     `json:"refresh_point"`
	PointAt         int64                     `json:"point_at"`
	Day             string                    `json:"day"`
	PhaseRecv       map[int]int64             `json:"phase_recv_bonus"`
	PhaseTotal      map[int]map[int]int64     `json:"phase_total_bonus"`
	BonusTime       int64                     `json:"bonus_time"`
	PolicyVersion   string                    `json:"policy_version"`
	GenerationCount int64                     `json:"generation_count"`
	PointRemainder  int64                     `json:"point_remainder"`
	RecoveryRate    int64                     `json:"recovery_rate"`
}

func consignProperties(p Progress, w *ConsignState) map[string]any {
	if w == nil {
		w = &ConsignState{Tasks: map[int]*ConsignTaskState{}, PhaseRecv: map[int]int64{}, PhaseTotal: map[int]map[int]int64{}}
	}
	return map[string]any{"consign_tasks": activityWire(w.Tasks), "consign_finish_times": w.FinishTimes, "consign_phase_recv_bonus": activityWire(w.PhaseRecv), "consign_phase_total_bonus": activityWire(w.PhaseTotal), "consign_bonus_time": w.BonusTime, "consign_refresh_point": map[string]any{"value": w.RefreshPoint, "last_time": w.PointAt, "max_limit": 5000, "interval": 3600, "per_value": consignRecoverRate(p)}, "consign_next_refresh_time": w.NextRefresh}
}

func ensureRemainingGameplay(p *Progress, now time.Time) error {
	w := ensureRemainingState(p)
	if w.Consign == nil {
		w.Consign = &ConsignState{Tasks: map[int]*ConsignTaskState{}, PhaseRecv: map[int]int64{}, PhaseTotal: map[int]map[int]int64{}, PointAt: now.Unix()}
	}
	return refreshConsignState(p, now)
}
