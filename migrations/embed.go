// Package migrations 通过 embed 把 SQL 迁移文件打包进二进制，供 cmd/migrate 执行。
package migrations

import "embed"

// FS 包含本目录下所有 SQL 迁移文件。
//
//go:embed *.sql
var FS embed.FS
