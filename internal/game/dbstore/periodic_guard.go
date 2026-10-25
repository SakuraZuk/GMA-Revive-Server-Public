package dbstore

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var errPeriodicDependencyBusy = errors.New("跨角色事务正在执行，周期结算稍后重试")

// 单角色改档由行版本保护；跨角色读写必须与周期写阶段互斥。
// 按数据库当前schema隔离，普通跨角色事务之间仍可并发。
const periodicDependencyKey = "hashtextextended(current_schema() || ':周期跨角色依赖',7143)"

func lockSocialDependency(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared("+periodicDependencyKey+")")
	return err
}

// 必须在可串行化快照建立之前取得保护锁。若在事务内等待锁，
// PostgreSQL可能先建立旧快照，锁到手后仍会读到等待前的跨角色状态。
// 使用独占池连接承载会话锁，任何释放异常都关闭物理连接，禁止锁泄漏回池。
func (s *Store) beginPeriodicWrite(ctx context.Context) (pgx.Tx, func(), error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	var locked bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock("+periodicDependencyKey+")").Scan(&locked); err != nil {
		_ = conn.Hijack().Close(context.Background())
		return nil, nil, err
	}
	if !locked {
		conn.Release()
		return nil, nil, errPeriodicDependencyBusy
	}
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var released bool
		if unlockErr := conn.QueryRow(cleanupCtx, "SELECT pg_advisory_unlock("+periodicDependencyKey+")").Scan(&released); unlockErr != nil || !released {
			_ = conn.Hijack().Close(cleanupCtx)
			return
		}
		conn.Release()
	}
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return tx, cleanup, nil
}
