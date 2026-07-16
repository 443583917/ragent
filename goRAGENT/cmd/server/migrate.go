package main

import (
	"flag"
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// runMigrations 执行数据库迁移
func runMigrations(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	sql, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("读取迁移文件失败: %w", err)
	}

	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	if err := db.Exec(string(sql)).Error; err != nil {
		return fmt.Errorf("执行迁移失败: %w", err)
	}

	fmt.Println("✅ 数据库迁移完成 (migrations/001_init.sql)")
	return nil
}

// HasMigrateFlag 检查是否传了 --migrate 参数
func HasMigrateFlag() bool {
	migrate := flag.Bool("migrate", false, "Run database migrations")
	flag.Parse()
	return *migrate
}
