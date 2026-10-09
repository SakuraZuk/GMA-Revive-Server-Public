package game

import (
	"encoding/json"
	"errors"
)

// 2A089B57：章节完成跳过支线3/7和活动9/10；地点下全部副本finished才算完成。
func remainingChapterFinished(p Progress, id int) bool {
	var chapters map[int]map[int][]int
	if json.Unmarshal(androidRemainingGameplay["chapter_site_dungeon"]["chapter_dict"], &chapters) != nil {
		return false
	}
	for site, dungeons := range chapters[id] {
		kind := remainingData("sites", site).integer("site_type")
		if kind == 3 || kind == 7 || kind == 9 || kind == 10 {
			continue
		}
		for _, did := range dungeons {
			if !containsInt(p.ClearedDungeons, did) {
				return false
			}
		}
	}
	return true
}
func remainingChapterUnlocked(p Progress, id int) error {
	row := remainingData("chapter", id)
	if len(row) == 0 {
		return errors.New("原生章节不存在")
	}
	var conditions [][][]json.RawMessage
	if json.Unmarshal(row["unlock_condition"], &conditions) != nil {
		return errors.New("章节前置条件格式无效")
	}
	for _, group := range conditions {
		if len(group) == 0 {
			continue
		}
		matched := false
		for _, condition := range group {
			if len(condition) != 2 {
				return errors.New("章节前置项格式无效")
			}
			var kind, value int
			if json.Unmarshal(condition[0], &kind) != nil {
				return errors.New("章节前置类型无效")
			}
			if kind == 3 {
				if json.Unmarshal(condition[1], &value) != nil {
					return errors.New("前置章节编号无效")
				}
				matched = matched || remainingChapterFinished(p, value)
			} else {
				ok, err := shopConditions([][][]json.RawMessage{{condition}}, p, p.AvatarLevel)
				if err != nil {
					return err
				}
				matched = matched || ok
			}
		}
		if !matched {
			return errors.New("前置章节尚未全部完成")
		}
	}
	return nil
}
