// internal/report/tabledoc.go
//
// 테이블정의서의 «레이아웃»을 한 곳에 둔다. MD·HTML·PDF·xlsx 넷이 이 파일의
// 것을 그대로 쓰고, 각 리포터는 그것을 자기 문법으로 찍기만 한다. 포맷마다
// 레이아웃을 따로 적으면 같은 실행에서 나온 산출물들이 서로 다른 문서가 된다
// — page_as_domain이 Markdown에만 걸려 있던 I6가 정확히 그 실패였다.
//
// 사내 표준 양식이 없다는 것을 확인하고(2026-09-04 소유자) 관례적인 정의서
// 레이아웃으로 짰다: 문서 머리 → 테이블 일람 → 테이블별 컬럼 표.
package report

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"erdtool/internal/buildinfo"
	"erdtool/internal/dialect"
	"erdtool/internal/drawio"
	"erdtool/internal/model"
)

// docTitle은 네 포맷이 공유하는 문서 제목이다.
const docTitle = "테이블정의서"

// Options는 리포터 넷이 공통으로 받는 것이다. 인자를 하나씩 늘리면 네 함수의
// 시그니처가 함께 길어지고, 호출자가 순서를 헷갈리는 순간 «pageAsDomain 자리에
// dialect가 들어가는» 사고가 난다.
type Options struct {
	// PageAsDomain은 페이지를 «도메인»이라 부를지다(스펙: 다중 페이지 처리).
	PageAsDomain bool
	// Dialect는 이 ERD가 겨누는 DBMS다. 빈 값이면 ANSI다.
	//
	// SQL을 안 뽑을 때도 뜻이 있다 — 정의서의 타입 칸이 어느 DB 것인지
	// 문서가 말해야 하고, 검증도 그 기준으로 돈다. `--sql`은 거기에
	// schema.sql 파일을 하나 더 내놓을 뿐이다.
	Dialect dialect.Dialect
}

// docHeaderLines는 문서 머리에 들어갈 «이름: 값» 줄들이다. 결재에 올릴
// 문서라면 어느 파일의 어느 시점을 무엇으로 뽑았는지가 문서 안에 있어야
// 한다 — 파일명만 보고는 알 수 없고, 메일에 붙어 오면 더 알 수 없다.
func docHeaderLines(doc model.Document, opt Options) [][2]string {
	// 경로가 아니라 파일 이름만 싣는다. 산출물 폴더는 원본 옆에 생기므로
	// 어느 폴더인지는 문서를 연 사람이 이미 알고 있고, 절대 경로를 그대로
	// 찍으면 그 한 줄이 문서 머리를 통째로 차지한다(실제로 그랬다).
	lines := [][2]string{{"원본", filepath.Base(doc.SourceFile)}}
	if doc.SourceModified != "" {
		// «뽑은 시각»이 아니라 «원본이 마지막으로 바뀐 시각»이다. 이유는
		// model.Document.SourceModified의 주석에 있다.
		lines = append(lines, [2]string{"기준 시각", doc.SourceModified})
	}
	// 타깃을 안 줬으면 «ANSI (지정 안 함)»이라고 적는다. 조용히 비우면
	// 「ANSI로 정한 것」인지 「깜빡한 것」인지 산출물만 봐서는 알 수 없다.
	lines = append(lines, [2]string{"타깃 DBMS", opt.Dialect.Label()})
	lines = append(lines, [2]string{"생성 도구", "erdtool " + buildinfo.Version})
	lines = append(lines, [2]string{"테이블 수", strconv.Itoa(countTables(doc)) + "개"})
	return lines
}

func countTables(doc model.Document) int {
	n := 0
	eachTable(doc, func(model.Diagram, model.Table, conflictIndex) { n++ })
	return n
}

// tableListHeaders는 테이블 일람의 열이다. 정의서를 여러 장 넘기기 전에
// 무엇이 있는지 한눈에 보는 표다.
var tableListHeaders = []string{"No", "테이블명", "설명", "컬럼 수", "페이지"}

// tableListRows는 일람의 본문을 만든다. 순번은 아래 테이블별 제목의 순번과
// 같다 — 문서 안에서 «몇 번»이 두 가지를 가리키면 서로 못 짚는다.
func tableListRows(doc model.Document, opt Options) [][]string {
	var rows [][]string
	n := 0
	eachTable(doc, func(d model.Diagram, t model.Table, i conflictIndex) {
		n++
		rows = append(rows, []string{
			strconv.Itoa(n),
			t.Name,
			attrCell(i, t.ID, drawio.AttrComment, t.Comment),
			strconv.Itoa(len(t.Columns)),
			pageLabel(d.Name, opt.PageAsDomain),
		})
	})
	return rows
}

// pageLabel은 페이지 칸의 값이다. page_as_domain이 켜지면 페이지를 «도메인»
// 이라 부른다(스펙: 다중 페이지 처리).
func pageLabel(pageName string, pageAsDomain bool) string {
	if pageName == "" {
		return ""
	}
	if pageAsDomain {
		return pageName + " (도메인)"
	}
	return pageName
}

// tableDocHeaders는 테이블별 컬럼 표의 열이다.
//
// PK와 FK를 따로 두는 것이 관례다. 다만 FK 칸에는 ● 대신 **키 셀의 원문**
// (FK1, FK2)을 넣는다 — 어느 관계에 딸린 외래키인지가 그 번호에 들어 있고,
// ●로 바꾸면 그 정보가 사라진다.
//
// 값이 없는 파일이라도 열은 그대로 둔다. 정의서 «양식»이 파일마다 모양을
// 바꾸면 결재에 올릴 수 없다.
var tableDocHeaders = []string{
	"No", "컬럼명", "데이터 타입", "PK", "FK", "참조", "NULL 허용", "기본값", "제약", "설명",
}

// relationHeaders는 **테이블 하나에 딸린** 관계 표의 열이다.
//
// 관계정의서(`--relations`)와 달리 이 표는 테이블마다 붙는다. 그래서 열이
// «이 테이블 기준»이다 — 상대가 누구고, 이쪽 컬럼과 저쪽 컬럼이 무엇이며,
// 이 테이블이 참조하는 쪽인지 참조당하는 쪽인지.
//
// 2026-09-04 소유자 결정: **관계정보는 그 테이블의 자리에 함께 적는다.**
// 엑셀은 그 테이블 탭 안에, PDF는 그 테이블 페이지 안에, Markdown은 그
// 테이블 절 안에. 예전에는 `--relations`를 줘야 나오는 별도 파일뿐이라,
// 기본 산출물만 받은 사람은 ERD의 절반을 못 봤다.
var relationHeaders = []string{
	"No", "방향", "상대 테이블", "이 테이블 컬럼", "상대 컬럼", "카디널리티(이쪽)", "카디널리티(상대)",
}

// 방향 칸의 값. 「미정」은 어느 쪽이 외래키인지 판정하지 못한 경우이며,
// 그 관계는 정의서의 참조 칸과 DDL의 FOREIGN KEY에도 안 실린다
// (검증의 relationship_without_fk가 같은 것을 짚는다).
const (
	dirRefersTo   = "→ 참조함"
	dirReferredBy = "← 참조됨"
	dirUnknown    = "미정"
)

// tableRelationRows는 이 테이블에 걸린 관계만 골라 «이 테이블 기준»으로
// 돌려준다. refs는 그 페이지의 «컬럼 -> 참조 대상» 표다.
//
// 해석되지 않은 끝점도 뺴지 않는다 — 조용히 빠지면 그림에는 선이 있는데
// 문서에는 흔적도 없어서, 사람은 문서가 맞는 줄 안다.
func tableRelationRows(d model.Diagram, t model.Table, refs map[string]model.ColumnRef) [][]string {
	tables := map[string]model.Table{}
	columns := map[string]model.Column{}
	for _, tt := range d.Tables {
		tables[tt.ID] = tt
		for _, c := range tt.Columns {
			columns[c.ID] = c
		}
	}

	var rows [][]string
	for _, r := range d.Relationships {
		// 이쪽/저쪽을 가른다. 자기 참조(양 끝이 같은 테이블)면 source가
		// 이쪽이다 — 그래야 한 관계가 표에 한 줄로만 나온다.
		var nearCol, farCol, farTable, nearCard, farCard string
		var farResolved bool
		var farRawID string
		switch {
		case r.SourceTableID == t.ID:
			nearCol, farCol = r.SourceColumnID, r.TargetColumnID
			farTable, farResolved, farRawID = r.TargetTableID, r.TargetResolved, r.TargetRawID
			nearCard, farCard = r.SourceCardinality, r.TargetCardinality
		case r.TargetTableID == t.ID:
			nearCol, farCol = r.TargetColumnID, r.SourceColumnID
			farTable, farResolved, farRawID = r.SourceTableID, r.SourceResolved, r.SourceRawID
			nearCard, farCard = r.TargetCardinality, r.SourceCardinality
		default:
			continue // 이 테이블과 무관한 관계
		}

		direction := dirUnknown
		if _, ok := refs[nearCol]; ok {
			direction = dirRefersTo
		} else if _, ok := refs[farCol]; ok {
			direction = dirReferredBy
		}

		rows = append(rows, []string{
			strconv.Itoa(len(rows) + 1),
			direction,
			endName(tables[farTable].Name, farResolved, farRawID),
			columns[nearCol].Name,
			columns[farCol].Name,
			cardinalityLabel(nearCard),
			cardinalityLabel(farCard),
		})
	}
	return rows
}

// noRelationNote는 관계가 하나도 없는 테이블의 자리에 들어갈 문구다. 표를
// 통째로 빼지 않는다 — 「관계가 없다」와 「이 문서가 관계를 안 싣는다」는
// 다른 말이고, 빈 자리는 후자로도 읽힌다.
const noRelationNote = "이 테이블에 걸린 관계선이 없다."

// conflictIndex는 «이 셀의 이 이름은 값을 고르지 않았다»를 조회한다.
// 키는 셀 id + 속성 이름이며, 컬럼 제약의 셀 id는 행(tableRow) 셀 id다(I7).
type conflictIndex map[string]bool

func newConflictIndex(d model.Diagram) conflictIndex {
	idx := conflictIndex{}
	for _, c := range d.AttrConflicts {
		idx[c.CellID+"\x00"+c.Name] = true
	}
	return idx
}

func (i conflictIndex) has(cellID, name string) bool { return i[cellID+"\x00"+name] }

// conflictNote는 충돌한 자리에 들어갈 문구다. **빈 칸으로 두지 않는다** —
// 「값을 안 적었다」와 「값을 적었는데 도구가 못 골랐다」는 사용자가 할 일이
// 전혀 다르고, 빈 칸은 전자로만 읽힌다.
const conflictNote = "(충돌 — 값을 고르지 않음)"

// attrCell은 속성 하나가 표에서 차지할 칸을 만든다.
func attrCell(i conflictIndex, cellID, name, value string) string {
	if i.has(cellID, name) {
		return conflictNote
	}
	return value
}

// pkMark는 PK 칸의 표시다. 관례대로 채움표 하나만 찍는다.
const pkMark = "●"

// columnRow는 컬럼 한 줄을 tableDocHeaders 순서대로 만든다. no는 표 안의
// 순번(1부터)이고, refs는 그 페이지의 «컬럼 -> 참조 대상» 표다.
func columnRow(no int, c model.Column, i conflictIndex, refs map[string]model.ColumnRef) []string {
	nullable := "Y"
	if !c.Nullable {
		nullable = "N"
	}
	pk := ""
	if c.IsPrimaryKey() {
		pk = pkMark
	}
	return []string{
		strconv.Itoa(no),
		c.Name,
		c.Type,
		pk,
		c.ForeignKeyTokens(),
		referenceCell(refs[c.ID]),
		nullable,
		attrCell(i, c.ID, drawio.AttrDefault, c.Default),
		constraintCell(c, i),
		attrCell(i, c.ID, drawio.AttrComment, c.Comment),
	}
}

// referenceCell은 「참조」 칸을 만든다. FK 칸의 `FK1`은 «외래키다»만 말하고
// 무엇을 가리키는지는 말하지 않는다 — 그 정보는 관계선에 있고, 정의서만
// 보는 사람에게는 이 칸이 없으면 닿지 않는다.
//
// 관계선이 행이 아니라 테이블에 직접 연결됐으면 컬럼 이름이 없다. 그때는
// 테이블만 적는다 — 없는 컬럼 이름을 지어내지 않는다.
func referenceCell(ref model.ColumnRef) string {
	switch {
	case ref.TableName == "":
		return ""
	case ref.ColumnName == "":
		return ref.TableName
	default:
		return ref.TableName + "." + ref.ColumnName
	}
}

// constraintCell은 「제약」 칸을 만든다. UNIQUE와 CHECK를 한 칸에 둔다 —
// UNIQUE만을 위한 열은 대부분의 표에서 빈 칸으로 남는다.
//
// 키 셀에 PK도 FK도 아닌 낱말이 적혀 있으면(오타 "P K" 같은 것) 그것도 여기
// 싣는다. PK/FK 두 칸으로 나눠 담는 순간 그런 값은 **표에서 아예 사라지는데**,
// 사라지면 사용자는 자기가 그 칸에 무엇을 썼는지 문서만 보고는 모른다.
func constraintCell(c model.Column, i conflictIndex) string {
	var parts []string
	if c.Unique {
		parts = append(parts, "UNIQUE")
	}
	if v := attrCell(i, c.ID, drawio.AttrCheck, c.Check); v != "" {
		if i.has(c.ID, drawio.AttrCheck) {
			parts = append(parts, v) // 충돌 문구는 그대로 둔다
		} else {
			parts = append(parts, "CHECK ("+v+")")
		}
	}
	if leftover := c.UnrecognizedKeyTokens(); leftover != "" {
		parts = append(parts, fmt.Sprintf("키 셀: %q", leftover))
	}
	return strings.Join(parts, ", ")
}

// tableHeading은 테이블 제목 한 줄을 만든다. 관례대로 순번을 앞에 붙인다 —
// 일람의 번호와 같은 번호라 서로 짚을 수 있다.
func tableHeading(no int, tableName string) string {
	return fmt.Sprintf("%d. %s", no, tableName)
}

// tableHeadingLines는 테이블 제목 아래에 붙는 줄들이다. 표의 열로 넣지 않는
// 이유는 그 값이 컬럼마다 있는 것이 아니라 테이블에 하나라서, 열로 만들면
// 모든 줄에 같은 값이 반복되거나 빈 칸이 되기 때문이다.
func tableHeadingLines(d model.Diagram, t model.Table, i conflictIndex, opt Options) []string {
	var notes []string
	if label := pageLabel(d.Name, opt.PageAsDomain); label != "" {
		notes = append(notes, fmt.Sprintf("페이지: %s", label))
	}
	if v := attrCell(i, t.ID, drawio.AttrComment, t.Comment); v != "" {
		notes = append(notes, fmt.Sprintf("설명: %s", v))
	}
	if v := attrCell(i, t.ID, drawio.AttrCheck, t.Check); v != "" {
		notes = append(notes, fmt.Sprintf("테이블 CHECK: %s", v))
	}
	return notes
}

// eachTable은 리포터 넷과 DDL 생성이 공통으로 도는 순회다. 도는 규칙이 여러
// 곳에 복제되면 한 곳만 고쳤을 때 산출물끼리 조용히 갈린다 — 관계정의서가
// 실제로 그랬다(M4).
func eachTable(doc model.Document, fn func(d model.Diagram, t model.Table, i conflictIndex)) {
	for _, d := range doc.Diagrams {
		i := newConflictIndex(d)
		for _, t := range d.Tables {
			fn(d, t, i)
		}
	}
}

// columnRows는 테이블 하나의 본문 줄들을 만든다.
func columnRows(t model.Table, i conflictIndex, refs map[string]model.ColumnRef) [][]string {
	rows := make([][]string, 0, len(t.Columns))
	for n, c := range t.Columns {
		rows = append(rows, columnRow(n+1, c, i, refs))
	}
	return rows
}

// eachTableWithRefs는 리포터 넷이 도는 순회다. 참조 표(refs)는 **페이지마다
// 한 번만** 만들어 넘긴다 — 테이블마다 다시 만들면 관계 전체를 테이블 수만큼
// 읽게 되어 제곱으로 느려진다.
func eachTableWithRefs(doc model.Document,
	fn func(d model.Diagram, t model.Table, no int, i conflictIndex, refs map[string]model.ColumnRef)) {

	no := 0
	for _, d := range doc.Diagrams {
		i := newConflictIndex(d)
		refs := d.ColumnReferences()
		for _, t := range d.Tables {
			no++
			fn(d, t, no, i, refs)
		}
	}
}
