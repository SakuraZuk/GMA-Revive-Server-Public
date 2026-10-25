package dbstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
)

// 周期事务仍在原有行锁内计算、整批提交。这里只缩小传给PG的JSON，
// 不提交预览快照，也不把跨角色结算拆成可能部分成功的事务。
type socialProgressPatch struct {
	set    []byte
	remove []string
}

func socialStatePatch(before, after []byte) (socialProgressPatch, error) {
	var old, current map[string]json.RawMessage
	if err := json.Unmarshal(before, &old); err != nil {
		return socialProgressPatch{}, err
	}
	if err := json.Unmarshal(after, &current); err != nil {
		return socialProgressPatch{}, err
	}
	if old == nil || current == nil {
		return socialProgressPatch{}, errors.New("周期进度差异必须为JSON对象")
	}
	changed := map[string]json.RawMessage{}
	removed := []string{}
	for key, value := range current {
		if previous, exists := old[key]; !exists || !bytes.Equal(previous, value) {
			changed[key] = value
		}
	}
	for key := range old {
		if _, exists := current[key]; !exists {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	raw, err := json.Marshal(changed)
	return socialProgressPatch{raw, removed}, err
}

func socialStatePatches(ctx context.Context, ids []string, original map[string][]byte, changed [][]byte, workers int) ([]socialProgressPatch, error) {
	patches := make([]socialProgressPatch, len(ids))
	workers = max(1, min(workers, len(ids)))
	failures := make([]error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for index := worker; index < len(ids); index += workers {
				if err := ctx.Err(); err != nil {
					failures[worker] = err
					return
				}
				if changed[index] == nil {
					continue
				}
				patch, err := socialStatePatch(original[ids[index]], changed[index])
				if err != nil {
					failures[worker] = err
					return
				}
				patches[index] = patch
			}
		}(worker)
	}
	wait.Wait()
	for _, err := range failures {
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return patches, nil
}
