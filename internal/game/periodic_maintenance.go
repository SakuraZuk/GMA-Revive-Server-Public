package game

import (
	"context"
	"fmt"
	"log"
	"time"
)

const periodicMaintenanceInterval = time.Minute
const periodicSettlementTimeout = 120 * time.Second

// StartPeriodicMaintenance在监听玩家请求前确定后台归属，避免启动竞态。
// 一旦交由后台，即使某轮失败或上下文取消，玩家请求也不得接管全服结算。
func (s *Service) StartPeriodicMaintenance(ctx context.Context) {
	if s.backgroundPeriodicStarted.CompareAndSwap(false, true) {
		go s.RunPeriodicMaintenance(ctx)
	}
}

// 仅周期纯计算可预览两次；社交操作和管理命令保持原单次事务契约。
func updatePeriodicSocial(ctx context.Context, store SocialAccounts, fn func(map[string]*Avatar) error) ([]Avatar, error) {
	if preview, ok := store.(interface {
		UpdatePeriodicSocial(context.Context, func(map[string]*Avatar) error) ([]Avatar, error)
	}); ok {
		return preview.UpdatePeriodicSocial(ctx, fn)
	}
	return store.UpdateAllSocial(ctx, fn)
}

// RunPeriodicMaintenance 是进程内唯一的全服周期结算入口。
// 它不占用玩家连接锁，活动与PVP顺序执行，失败时不推进各自的幂等周期键。
func (s *Service) RunPeriodicMaintenance(ctx context.Context) {
	s.backgroundPeriodicStarted.Store(true)
	if err := s.RunPeriodicMaintenanceOnce(ctx); err != nil {
		log.Printf("后台周期结算失败，将在一分钟后重试：%v", err)
	}
	ticker := time.NewTicker(periodicMaintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunPeriodicMaintenanceOnce(ctx); err != nil {
				log.Printf("后台周期结算失败，将在一分钟后重试：%v", err)
			}
		}
	}
}

// RunPeriodicMaintenanceOnce 执行一次可验收的后台维护。
// 活动结算失败时不会继续启动另一轮PVP全表事务，下一分钟从活动重新尝试。
func (s *Service) RunPeriodicMaintenanceOnce(ctx context.Context) error {
	activityCtx, cancelActivity := context.WithTimeout(ctx, periodicSettlementTimeout)
	ready, err := s.tryRefreshActivityAwards(activityCtx)
	cancelActivity()
	if err != nil {
		return fmt.Errorf("活动排行结算：%w", err)
	}
	if !ready {
		return nil
	}

	pvpCtx, cancelPvp := context.WithTimeout(ctx, periodicSettlementTimeout)
	ready, err = s.tryRefreshPvpAwards(pvpCtx, nil)
	cancelPvp()
	if err != nil {
		return fmt.Errorf("竞技与助战周期结算：%w", err)
	}
	_ = ready // 忙时不推进周期键，下一分钟重试。
	return nil
}

// acceptPeriodicRows 让前台调用更新当前连接快照；后台调用只通知在线角色回载。
func (s *Service) acceptPeriodicRows(c *Connection, rows []Avatar) {
	if c != nil {
		acceptSocialRows(c, rows)
		return
	}
	for _, av := range rows {
		s.publishPlayerRefresh(av.OID)
	}
}
