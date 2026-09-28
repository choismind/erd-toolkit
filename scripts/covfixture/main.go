// scripts/covfixture는 scripts/coverage.sh가 쓸 픽스처 둘을 만든다.
//
// 저장소에 두지 않고 만들어 쓰는 이유: 표준용어사전은 xlsx라 텍스트로 적을
// 수 없고, SQLite는 DB 파일이라 마찬가지다. 둘 다 내용이 몇 줄뿐이라
// 바이너리를 커밋하는 것보다 만드는 코드를 두는 편이 낫다 — 무엇이 들어
// 있는지 열어 보지 않아도 알 수 있다.
//
// 이 둘이 있어야 convert와 reverse의 «잘 되는 길»을 잴 수 있다. 없으면
// 그 둘은 실패 경로만 도는데, 그것만으로는 runConvert가 절반도 안 덮인다.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xuri/excelize/v2"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		die(fmt.Errorf("usage: covfixture <폴더>"))
	}
	dir := os.Args[1]
	if err := writeDict(filepath.Join(dir, "표준용어사전.xlsx")); err != nil {
		die(err)
	}
	if err := writeDB(filepath.Join(dir, "shop.db")); err != nil {
		die(err)
	}
}

// writeDict는 표준용어사전을 만든다. 첫 시트가 용어사전, 둘째가 단어사전이며
// 순서로 정해진다(시트 이름은 보지 않는다).
func writeDict(path string) error {
	f := excelize.NewFile()
	defer f.Close()

	terms := [][2]string{{"고객번호", "CUST_NO"}, {"고객명", "CUST_NM"}, {"주문번호", "ORDR_NO"}}
	words := [][2]string{{"고객", "CUST"}, {"주문", "ORDR"}, {"번호", "NO"}, {"명", "NM"}}

	// 기본 시트가 용어사전이 된다.
	first := f.GetSheetName(0)
	if err := fill(f, first, terms); err != nil {
		return err
	}
	if _, err := f.NewSheet("단어사전"); err != nil {
		return err
	}
	if err := fill(f, "단어사전", words); err != nil {
		return err
	}
	return f.SaveAs(path)
}

func fill(f *excelize.File, sheet string, rows [][2]string) error {
	if err := f.SetCellStr(sheet, "A1", "한글"); err != nil {
		return err
	}
	if err := f.SetCellStr(sheet, "B1", "영문약어"); err != nil {
		return err
	}
	for i, r := range rows {
		n := i + 2
		if err := f.SetCellStr(sheet, fmt.Sprintf("A%d", n), r[0]); err != nil {
			return err
		}
		if err := f.SetCellStr(sheet, fmt.Sprintf("B%d", n), r[1]); err != nil {
			return err
		}
	}
	return nil
}

// writeDB는 reverse가 읽을 SQLite를 만든다. 테이블 둘에 FK 하나면
// 카디널리티 판정까지 한 번씩 탄다.
func writeDB(path string) error {
	os.Remove(path) // 두 번 돌려도 같은 결과가 나와야 한다
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`
CREATE TABLE customer (
  id    INTEGER PRIMARY KEY,
  name  TEXT NOT NULL,
  email TEXT UNIQUE
);
CREATE TABLE orders (
  id          INTEGER PRIMARY KEY,
  customer_id INTEGER NOT NULL REFERENCES customer(id),
  ordered_on  TEXT NOT NULL
);`)
	return err
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
