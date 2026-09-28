package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"erdtool/internal/config"
	"erdtool/internal/dbreverse"
	"erdtool/internal/genbuild"
)

type ReverseResult struct {
	Target       string // 마스킹된 표시용. 여기에 비밀번호가 있으면 안 된다.
	OutputPath   string
	Overwritten  bool
	Pages        int
	Tables       int
	Relations    int
	SkippedCross []string
}

// ReverseOutputPath는 접속 대상에서 기본 출력 경로를 만든다.
// SQLite는 DB 파일 옆에, 서버는 작업 디렉터리에 놓는다.
func ReverseOutputPath(t dbreverse.Target) string {
	if t.Driver == dbreverse.DriverSQLite {
		ext := filepath.Ext(t.DSN)
		return strings.TrimSuffix(t.DSN, ext) + ".drawio"
	}
	return t.Label + ".drawio"
}

// Reverse는 DB 하나를 읽어 .drawio를 쓴다.
//
//	arg      위치 인자 (섹션 이름 / DSN / SQLite 파일 경로)
//	outPath  비었으면 ReverseOutputPath를 쓴다
//	connPath 비었으면 작업 디렉터리에서 자동탐색한다
func Reverse(arg, outPath, connPath string) (ReverseResult, error) {
	conns, err := loadConnectionsFor(connPath)
	if err != nil {
		return ReverseResult{}, err
	}

	target, err := dbreverse.Resolve(arg, conns)
	if err != nil {
		return ReverseResult{}, err
	}

	dialect, err := dbreverse.DialectFor(target.Driver)
	if err != nil {
		return ReverseResult{}, err
	}

	// 접속보다 «덮어쓰기 가드»를 먼저 본다. 막을 일이면 DB를 건드리기 전에
	// 막는 것이 맞고, 실패 문구가 접속 에러에 묻히지도 않는다.
	if outPath == "" {
		outPath = ReverseOutputPath(target)
	}
	// SQLite 파일 위에 .drawio를 덮어쓰면 DB가 사라지고 되돌릴 길이 없다.
	// build가 설계서를 지키려고 넣은 가드와 같은 이유다.
	if target.Driver == dbreverse.DriverSQLite {
		if err := guardNotOverwriting(target.DSN, outPath); err != nil {
			return ReverseResult{}, err
		}
	}

	db, err := sql.Open(target.Driver, target.DSN)
	if err != nil {
		// target.DSN이 아니라 Display를 쓴다 — 원문에는 비밀번호가 있다.
		// DecodeError는 서버가 UTF-8이 아닌 로케일 인코딩으로 보낸 문구를
		// 읽을 수 있게 고친다 — 접속에 실패한 순간이 가장 중요한 순간이다.
		return ReverseResult{}, fmt.Errorf("접속 실패 (%s): %w", target.Display, dbreverse.DecodeError(err))
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return ReverseResult{}, fmt.Errorf("접속 실패 (%s): %w", target.Display, dbreverse.DecodeError(err))
	}

	schemas, err := dbreverse.Introspect(ctx, dialect, db)
	if err != nil {
		return ReverseResult{}, fmt.Errorf("%s: %w", target.Display, dbreverse.DecodeError(err))
	}

	pr := dbreverse.ToPageDefs(schemas)
	out, err := genbuild.Build(pr.Pages)
	if err != nil {
		return ReverseResult{}, fmt.Errorf("%s: %w", target.Display, err)
	}

	_, statErr := os.Stat(outPath)
	overwritten := statErr == nil

	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return ReverseResult{}, fmt.Errorf("출력 쓰기 실패: %w", err)
	}

	res := ReverseResult{
		Target: target.Display, OutputPath: outPath, Overwritten: overwritten,
		Pages: len(pr.Pages), SkippedCross: pr.CrossSchemaFKs,
	}
	for _, p := range pr.Pages {
		res.Tables += len(p.Tables)
		res.Relations += len(p.Relations)
	}
	return res, nil
}

// loadConnectionsFor는 명시된 경로를 쓰거나, 없으면 작업 디렉터리에서
// 자동탐색한다. 둘 다 없으면 «빈 설정»으로 계속한다 — DSN이나 파일 경로를
// 직접 준 경우에는 설정 파일이 필요 없기 때문이다.
func loadConnectionsFor(connPath string) (dbreverse.Connections, error) {
	if connPath == "" {
		wd, err := os.Getwd()
		if err != nil {
			return dbreverse.Connections{}, err
		}
		found, ok := config.DiscoverConnections(wd)
		if !ok {
			return dbreverse.Connections{}, nil
		}
		connPath = found
	}
	return dbreverse.LoadConnections(connPath)
}
