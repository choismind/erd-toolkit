package dbreverse

import (
	"fmt"
	"sort"
	"strings"

	"erdtool/internal/genbuild"
)

// PageResult는 변환 결과다. 그리지 못한 관계를 함께 돌려주는 것이 요점이다 —
// 조용히 버리면 산출물이 「관계가 원래 없었다」고 거짓말하게 된다.
type PageResult struct {
	Pages          []genbuild.PageDef
	CrossSchemaFKs []string
}

// ToPageDefs는 읽어들인 스키마들을 genbuild가 그릴 수 있는 모양으로 옮긴다.
// 스키마 하나가 페이지 하나다(스펙: 스키마 = 페이지).
//
// 이 함수는 순수하다 — DB도 파일도 만지지 않는다. 그래서 세 DBMS의 의미
// 매핑 전체를 서버 없이 테스트할 수 있다.
func ToPageDefs(schemas []Schema) PageResult {
	var res PageResult
	for _, s := range schemas {
		page, cross := toPage(s)
		res.Pages = append(res.Pages, page)
		res.CrossSchemaFKs = append(res.CrossSchemaFKs, cross...)
	}
	return res
}

func toPage(s Schema) (genbuild.PageDef, []string) {
	// 컬럼 조회용 색인. FK가 가리키는 컬럼이 실제로 있는지 확인해야 한다 —
	// genbuild.emitPage는 없으면 하드 에러를 낸다.
	colOf := map[string]map[string]Column{}
	pkSizeOf := map[string]int{} // 테이블 -> PK를 이루는 컬럼 개수
	for _, t := range s.Tables {
		colOf[t.Name] = map[string]Column{}
		for _, c := range t.Columns {
			colOf[t.Name][c.Name] = c
			if c.PK {
				pkSizeOf[t.Name]++
			}
		}
	}

	// 테이블별 FK 번호. 제약 이름 오름차순으로 1부터 붙인다.
	fkIndexOf := map[string]map[string]int{} // 테이블 -> 컬럼 -> FK번호
	var sameSchema []ForeignKey
	var cross []string
	for _, fk := range s.ForeignKeys {
		if fk.TargetSchema != "" && fk.TargetSchema != s.Name {
			for _, pair := range fk.Columns {
				cross = append(cross, fmt.Sprintf("%s.%s.%s -> %s.%s.%s",
					s.Name, fk.Table, pair.Column,
					fk.TargetSchema, fk.TargetTable, pair.TargetColumn))
			}
			continue
		}
		sameSchema = append(sameSchema, fk)
	}
	sort.SliceStable(sameSchema, func(i, j int) bool { return sameSchema[i].Name < sameSchema[j].Name })
	counter := map[string]int{}
	for _, fk := range sameSchema {
		counter[fk.Table]++
		n := counter[fk.Table]
		if fkIndexOf[fk.Table] == nil {
			fkIndexOf[fk.Table] = map[string]int{}
		}
		for _, pair := range fk.Columns {
			fkIndexOf[fk.Table][pair.Column] = n
		}
	}

	page := genbuild.PageDef{Name: s.Name}
	for _, t := range s.Tables {
		td := genbuild.TableDef{Name: t.Name}
		for _, c := range t.Columns {
			td.Columns = append(td.Columns, genbuild.ColumnDef{
				Name:     c.Name,
				Type:     c.Type,
				Key:      keyMarker(c, fkIndexOf[t.Name][c.Name]),
				Nullable: c.Nullable,
				Unique:   c.Unique,
			})
		}
		page.Tables = append(page.Tables, td)
	}

	seq := 0
	for _, fk := range sameSchema {
		unique := fkUnique(fk, colOf[fk.Table], pkSizeOf[fk.Table])
		for _, pair := range fk.Columns {
			childCol, ok := colOf[fk.Table][pair.Column]
			if !ok {
				continue
			}
			if _, ok := colOf[fk.TargetTable][pair.TargetColumn]; !ok {
				continue
			}
			seq++
			card := DecideCardinality(!childCol.Nullable, unique)
			page.Relations = append(page.Relations, genbuild.RelationDef{
				Seq:               seq,
				SourceTable:       fk.TargetTable,
				SourceColumn:      pair.TargetColumn,
				SourceCardinality: card.Parent,
				SourceKey:         keyMarker(colOf[fk.TargetTable][pair.TargetColumn], fkIndexOf[fk.TargetTable][pair.TargetColumn]),
				RelationName:      "",
				TargetCardinality: card.Child,
				TargetKey:         keyMarker(childCol, fkIndexOf[fk.Table][pair.Column]),
				TargetTable:       fk.Table,
				TargetColumn:      pair.Column,
			})
		}
	}
	return page, cross
}

// keyMarker는 컬럼의 키 표시를 만든다. PK이면서 FK면 "PK,FK1"이 된다 —
// 이 표기는 커밋된 픽스처(internal/drawio/testdata)에 실제로 있는 값이다.
func keyMarker(c Column, fkIndex int) string {
	var parts []string
	if c.PK {
		parts = append(parts, "PK")
	}
	if fkIndex > 0 {
		parts = append(parts, fmt.Sprintf("FK%d", fkIndex))
	}
	return strings.Join(parts, ",")
}

// fkUnique는 FK 컬럼 집합이 유일한지 본다 — 즉 「부모 하나에 자식이 최대
// 하나」를 DB가 실제로 강제하는지다.
//
// PK 참여만으로는 부족하다. PK가 (id, customer_id) 복합인데 FK가
// customer_id 하나뿐이면 그 컬럼만으로 자식 행이 하나로 정해지지 않는다.
// 그래서 **FK 컬럼 집합이 PK 전체를 덮을 때만** 유일로 본다. 단일 컬럼
// UNIQUE 제약은 그 자체로 유일하다.
//
// 애매하면 «유일하지 않다» 쪽으로 둔다. 그쪽이 ERzeroToMany(0개 이상)라
// 아무것도 주장하지 않는 쪽이기 때문이다.
func fkUnique(fk ForeignKey, cols map[string]Column, pkSize int) bool {
	if len(fk.Columns) == 1 {
		c := cols[fk.Columns[0].Column]
		return c.Unique || (c.PK && pkSize == 1)
	}
	if len(fk.Columns) != pkSize {
		return false
	}
	for _, pair := range fk.Columns {
		if !cols[pair.Column].PK {
			return false
		}
	}
	return true
}
