package game

// Android71BB353A注册handle_error_msg(Int,Tuple)，其内部调用本地show_error_tips。
// 不能将普通本地方法直接作为网络推送；无格式化参数仍必须传空Tuple。
func nativeErrorPush(code int) Push {
	return push("Avatar", "handle_error_msg", code, []any{})
}
