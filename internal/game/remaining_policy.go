package game

import "os"

// 此版本只对应用户可审核的余项政策提案；默认关闭。
// 正式部署只能在用户明确采用该版本服政策后配置环境变量。
// 客户端数据中已证明的接口/数值修复不受本政策开关影响。
func remainingPolicyEnabled() bool {
	return os.Getenv("HS_REMAINING_GAMEPLAY_POLICY") == "local-20261008-v1"
}
