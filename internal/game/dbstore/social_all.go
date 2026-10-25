package dbstore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"hs-server/internal/game"
	"log"
	"runtime"
	"sync"
	"time"
)

// 非周期全量管理先顺序锁全部avatars，再顺序锁全部progress；仅保存变动行。
func (s *Store) UpdateAllSocial(ctx context.Context, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	return s.updateAllSocial(ctx, fn, false, nil)
}

// 预读只缓存结算前的独立进度；xmin同时识别未递增业务revision的维护写入。
type periodicProgressSnapshot struct {
	original []byte
	revision int64
	version  string
}

// 周期纯计算先在一致只读快照中比较；没有变动时不获取全服行锁。
// 有变动时在可串行化事务的同一快照重新计算，只重新读取版本变化的完整进度。
// 写阶段先与跨角色事务互斥，再建立快照；普通单角色写入由行版本冲突保护。
// 不预先获取全服行锁；冲突整轮回滚，交由后台重试。
// 原始快照与预览计算不共享容器，禁止提交预览中的旧存档。
func (s *Store) UpdatePeriodicSocial(ctx context.Context, fn func(map[string]*game.Avatar) error) ([]game.Avatar, error) {
	started := time.Now()
	cache := map[string]periodicProgressSnapshot{}
	rows, err := s.updateAllSocial(ctx, fn, true, cache)
	if err != nil || len(rows) == 0 {
		log.Printf("周期存档预读结束：角色数=%d 预读耗时=%s 无需加锁=%t 成功=%t", len(cache), time.Since(started), err == nil, err == nil)
		return rows, err
	}
	locked := time.Now()
	rows, err = s.updateAllSocial(ctx, fn, false, cache)
	log.Printf("周期存档预读完成：角色数=%d 预读耗时=%s 写阶段耗时=%s 变动角色=%d 成功=%t", len(cache), locked.Sub(started), time.Since(locked), len(rows), err == nil)
	return rows, err
}

func (s *Store) updateAllSocial(ctx context.Context, fn func(map[string]*game.Avatar) error, preview bool, cache map[string]periodicProgressSnapshot) ([]game.Avatar, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	options := pgx.TxOptions{}
	lock := " FOR UPDATE"
	if preview {
		options = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
		lock = ""
	} else if cache != nil {
		options = pgx.TxOptions{IsoLevel: pgx.Serializable}
		lock = ""
	}
	var tx pgx.Tx
	var err error
	if !preview && cache != nil {
		var cleanup func()
		tx, cleanup, err = s.beginPeriodicWrite(ctx)
		if err == nil {
			defer cleanup()
		}
	} else {
		tx, err = s.pool.BeginTx(ctx, options)
	}
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout = '120s'`); err != nil {
		return nil, err
	}
	// JSON解析/排名计算发生在应用侧，不能被默认短事务空闲期限提前中断。
	// 仅限本事务，仍受外层120秒总期限约束，不更改数据库全局设置。
	if _, err = tx.Exec(ctx, `SET LOCAL idle_in_transaction_session_timeout = '120s'`); err != nil {
		return nil, err
	}
	if !preview && cache == nil {
		if err = lockSocialDependency(ctx, tx); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT avatar_oid,uid,hostnum,nickname,level,head_id,head_box_id,custom_head_image_url,account,gender,nickname_set,created_at FROM avatars ORDER BY avatar_oid`+lock)
	if err != nil {
		return nil, err
	}
	avatars := map[string]*game.Avatar{}
	ids := []string{}
	for rows.Next() {
		var av game.Avatar
		if err = rows.Scan(&av.OID, &av.UID, &av.Hostnum, &av.Info.Nickname, &av.Info.Level, &av.Info.HeadID, &av.Info.HeadBoxID, &av.Info.CustomHeadImageURL, &av.Account, &av.Gender, &av.NicknameSet, &av.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		id := hex.EncodeToString(av.OID)
		avatars[id] = &av
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	stateColumn := "state"
	if !preview && cache != nil {
		stateColumn = "NULL::jsonb"
	}
	rows, err = tx.Query(ctx, `SELECT avatar_oid,revision,xmin::text,`+stateColumn+` FROM avatar_progress ORDER BY avatar_oid`+lock)
	if err != nil {
		return nil, err
	}
	original := map[string][]byte{}
	reload := []string{}
	for rows.Next() {
		var oid, raw []byte
		var revision int64
		var version string
		if err = rows.Scan(&oid, &revision, &version, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		id := hex.EncodeToString(oid)
		av := avatars[id]
		if av == nil {
			rows.Close()
			return nil, errors.New("周期进度存在孤立角色")
		}
		if !preview && cache != nil {
			prior, ok := cache[id]
			if !ok || prior.revision != revision || prior.version != version {
				reload = append(reload, id)
				continue
			}
			// 只保留不可变JSON快照；预览期不能同时常驻两份全服解码进度。
			// 写阶段从原始快照重新解码，绝不复用预览回调已修改的容器。
			if err = json.Unmarshal(prior.original, &av.Progress); err != nil {
				rows.Close()
				return nil, err
			}
			original[id] = prior.original
			if av.Progress.AvatarLevel != av.Info.Level {
				av.Progress.AvatarLevel = av.Info.Level
				original[id], err = json.Marshal(av.Progress)
				if err != nil {
					rows.Close()
					return nil, err
				}
			}
			continue
		}
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			rows.Close()
			return nil, err
		}
		av.Progress.AvatarLevel = av.Info.Level
		original[id], err = json.Marshal(av.Progress)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if preview && cache != nil {
			cache[id] = periodicProgressSnapshot{original[id], revision, version}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// 先消费并关闭锁定游标，再读取变动行，避免同一连接上嵌套查询。
	for _, id := range reload {
		av := avatars[id]
		var raw []byte
		if err = tx.QueryRow(ctx, `SELECT state FROM avatar_progress WHERE avatar_oid=$1`, av.OID).Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &av.Progress); err != nil {
			return nil, err
		}
		av.Progress.AvatarLevel = av.Info.Level
		original[id], err = json.Marshal(av.Progress)
		if err != nil {
			return nil, err
		}
	}
	// 早期版本曾留下“角色主表存在、进度表缺失”的遗留行。全服排行与周期
	// 结算只能使用有完整进度的角色；单个遗留行不得阻断所有正常玩家登录、
	// 教学副本或活动结算。这里在同一只读快照中隔离它们，不猜测或补造进度。
	validIDs := ids[:0]
	for _, id := range ids {
		if _, exists := original[id]; !exists {
			delete(avatars, id)
			continue
		}
		validIDs = append(validIDs, id)
	}
	ids = validIDs
	if preview {
		// 全部一致快照已经读取完毕，纯计算不再保留数据库事务。
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
	}
	if err = fn(avatars); err != nil {
		return nil, err
	}
	// 周期回调已结束，进度不再被修改；仅编码与差异比较并行。
	// 非周期操作继续串行。输出只保留变化行，避免额外保留全服JSON副本。
	workers := 1
	if cache != nil && len(ids) >= 8 {
		workers = min(4, runtime.GOMAXPROCS(0))
	}
	changed, err := marshalSocialChanges(ctx, avatars, ids, original, workers)
	if err != nil {
		return nil, err
	}
	var patches []socialProgressPatch
	if cache != nil && !preview {
		patches, err = socialStatePatches(ctx, ids, original, changed, workers)
		if err != nil {
			return nil, err
		}
	}
	batch := &pgx.Batch{}
	out := []game.Avatar{}
	for index, id := range ids {
		av := avatars[id]
		raw := changed[index]
		if raw == nil {
			continue
		}
		if patches != nil {
			patch := patches[index]
			batch.Queue(`UPDATE avatar_progress SET state=(state - $3::text[]) || $2::jsonb,revision=revision+1,updated_at=now() WHERE avatar_oid=$1`, av.OID, string(patch.set), patch.remove)
		} else {
			batch.Queue(`UPDATE avatar_progress SET state=$2::jsonb,revision=revision+1,updated_at=now() WHERE avatar_oid=$1`, av.OID, string(raw))
		}
		if av.Progress.AvatarLevel != av.Info.Level {
			batch.Queue(`UPDATE avatars SET level=$2 WHERE avatar_oid=$1`, av.OID, av.Progress.AvatarLevel)
			av.Info.Level = av.Progress.AvatarLevel
		}
		out = append(out, *av)
	}
	if batch.Len() > 0 && !preview {
		if cache != nil {
			// 只锁本轮实际变动角色，仍遵守所有玩家写路径的avatars→progress顺序。
			// FOR UPDATE在可串行化快照中遇到并发改档会失败，不能覆盖新版本。
			changedIDs := make([][]byte, len(out))
			for i := range out {
				changedIDs[i] = out[i].OID
			}
			lockedRows, lockErr := tx.Query(ctx, `SELECT avatar_oid FROM avatars WHERE avatar_oid=ANY($1) ORDER BY avatar_oid FOR UPDATE`, changedIDs)
			if lockErr != nil {
				return nil, lockErr
			}
			for lockedRows.Next() {
			}
			lockErr = lockedRows.Err()
			lockedRows.Close()
			if lockErr != nil {
				return nil, lockErr
			}
		}
		results := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err = results.Exec(); err != nil {
				_ = results.Close()
				return nil, err
			}
		}
		if err = results.Close(); err != nil {
			return nil, err
		}
	}
	if !preview {
		if err = tx.Commit(ctx); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// 每个工作协程独占结果槽位；全部退出后才允许入批次或更新角色资料。
// 任一编码失败/取消都返回错误，调用者回滚整笔事务，不提交部分结果。
func marshalSocialChanges(ctx context.Context, avatars map[string]*game.Avatar, ids []string, original map[string][]byte, workers int) ([][]byte, error) {
	changed := make([][]byte, len(ids))
	workers = max(1, min(workers, len(ids)))
	errorsByWorker := make([]error, workers)
	var wait sync.WaitGroup
	encode := func(worker int) {
		for index := worker; index < len(ids); index += workers {
			if err := ctx.Err(); err != nil {
				errorsByWorker[worker] = err
				return
			}
			id := ids[index]
			raw, err := json.Marshal(avatars[id].Progress)
			if err != nil {
				errorsByWorker[worker] = err
				return
			}
			if !bytes.Equal(raw, original[id]) {
				changed[index] = raw
			}
		}
	}
	if workers == 1 {
		encode(0)
	} else {
		for worker := 0; worker < workers; worker++ {
			wait.Add(1)
			go func(worker int) {
				defer wait.Done()
				encode(worker)
			}(worker)
		}
		wait.Wait()
	}
	for _, err := range errorsByWorker {
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return changed, nil
}
