// Package db 保存唯一的账号角色建表定义，避免 SQL 文件与业务代码重复维护。
package db

import _ "embed"

// Schema 在启动时由 dbstore 在事务中应用。
//
//go:embed schema.sql
var Schema string
