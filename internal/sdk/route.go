// Package sdk 提供 SDK 管理路由的纯函数逻辑；它不承担网易原生 SDK 鉴权。
package sdk

import "strings"

type Route struct {
	Mode    string `json:"mode"`
	Address string `json:"address"`
	Reason  string `json:"reason"`
}

// SelectRoute 按客户端上报的版本选择热更服；未命中时始终返回游戏服。
// versions 是逗号分隔的精确版本号。force 仅供运维紧急切换使用。
func SelectRoute(version, versions, gameAddress, hotfixAddress string, force bool) Route {
	if force || containsVersion(version, versions) {
		return Route{Mode: "hotfix", Address: hotfixAddress, Reason: "版本命中独立热更服务器"}
	}
	return Route{Mode: "game", Address: gameAddress, Reason: "版本未命中热更服务器"}
}

func containsVersion(version, versions string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	for _, item := range strings.Split(versions, ",") {
		if strings.TrimSpace(item) == version {
			return true
		}
	}
	return false
}
