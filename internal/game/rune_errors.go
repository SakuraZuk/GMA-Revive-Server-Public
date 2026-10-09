package game

// 业务错误用名称从 Android 导出表取码，禁止将存储故障伪装为业务拒绝。
type runeBusinessError struct {
	Name    string
	Message string
}

func (e *runeBusinessError) Error() string { return e.Message }

func runeReject(name, message string) error {
	return &runeBusinessError{Name: name, Message: message}
}
