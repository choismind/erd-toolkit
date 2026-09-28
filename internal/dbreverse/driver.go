// Package dbreverse는 실행 중인 DB의 카탈로그를 읽어 genbuild.PageDef로
// 옮긴다. .drawio를 읽는 internal/drawio와 반대 방향이고, 만드는 쪽인
// internal/genbuild에 붙는다.
package dbreverse

import (
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// database/sql에 등록되는 드라이버 이름이다. 각 패키지가 init에서 등록하는
// 문자열과 정확히 같아야 하므로 손으로 짓지 않는다.
const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "pgx"
	DriverMySQL    = "mysql"
)
