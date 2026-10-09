package database

import (
	"context"
	"testing"
)

func TestOpenPostgresRequiresConfiguration(t *testing.T) {
	if _, err := OpenPostgres(context.Background(), "", 32); err == nil {
		t.Fatal("未配置数据库时必须快速失败")
	}
}

func TestOpenPostgresRejectsInvalidURL(t *testing.T) {
	if _, err := OpenPostgres(context.Background(), "://invalid", 32); err == nil {
		t.Fatal("数据库连接串非法时必须快速失败")
	}
}
