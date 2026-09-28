package convert

import (
	"os"
	"path/filepath"
	"testing"

	"erdtool/internal/drawio"
	"erdtool/internal/glossary"
)

const logicalDrawio = `<mxfile host="test"><diagram name="논리" id="p1"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1"/>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1"/>
  <mxCell id="r1d" value="고객번호 int" style="shape=partialRectangle;" vertex="1" parent="r1"/>
</root></mxGraphModel></diagram></mxfile>`

func fileTestDict(t *testing.T) *glossary.Dict {
	t.Helper()
	return glossary.MustLoadForTest(
		map[string]string{"고객번호": "CUST_NO"},
		map[string]string{"고객": "CUST", "번호": "NO"},
	)
}

func cellsOf(t *testing.T, data []byte) map[string]drawio.RawCell {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.drawio")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ds, err := drawio.LoadDiagrams(p)
	if err != nil {
		t.Fatalf("LoadDiagrams: %v", err)
	}
	out := map[string]drawio.RawCell{}
	for _, d := range ds {
		for _, c := range d.Cells {
			out[c.ID] = c
		}
	}
	return out
}

func TestFile_ConvertsNames(t *testing.T) {
	out, stats, err := File([]byte(logicalDrawio), fileTestDict(t))
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	cells := cellsOf(t, out)
	if got := cells["t1"].Value; got != "CUST" {
		t.Fatalf("t1.Value = %q; want CUST", got)
	}
	if got := cells["r1d"].Value; got != "CUST_NO int" {
		t.Fatalf("r1d.Value = %q; want 'CUST_NO int'", got)
	}
	if got := cells["r1k"].Value; got != "PK" {
		t.Fatalf("키 셀이 바뀌었다: %q", got)
	}
	if stats.Total != 2 || stats.Converted != 2 {
		t.Fatalf("stats = %+v; want Total=2 Converted=2", stats)
	}
}

func TestFile_IsIdempotent(t *testing.T) {
	d := fileTestDict(t)
	once, _, err := File([]byte(logicalDrawio), d)
	if err != nil {
		t.Fatalf("첫 변환: %v", err)
	}
	twice, _, err := File(once, d)
	if err != nil {
		t.Fatalf("두 번째 변환: %v", err)
	}
	if string(once) != string(twice) {
		t.Fatalf("두 번 돌린 결과가 다르다\n1회:\n%s\n2회:\n%s", once, twice)
	}
}

// TestFile_BoundaryWhitespaceAndUnderscore는 소유자 결정 둘이 파일
// 단위에서도 성립하는지, 그리고 그 결과가 멱등적인지 본다.
//
//	«맨 앞뒤 공백은 제거한 다음 변환» — "주문고객번호 "는 뒤에 붙은
//	   공백 하나 때문에 통째로 변환에 실패해 한글이 그대로 물리 ERD에
//	   실렸다. 그것을 «미매칭»이라고만 보고했으므로 사용자가 눈으로
//	   잡아내지 못하면 조용히 틀린 산출물이 된다.
//	«명칭에 있는 언더바는 지우지 않는다» — "_고객"의 앞 밑줄이
//	   소리 없이 사라져 "CUST"가 됐다.
//
// 멱등성은 logicalName이 지킨다: 2회차의 변환 소스는 언제나 저장된 한글
// 원문이므로 같은 바이트가 나와야 한다.
func TestFile_BoundaryWhitespaceAndUnderscore(t *testing.T) {
	d := glossary.MustLoadForTest(
		map[string]string{"고객번호": "CUST_NO"},
		map[string]string{"고객": "CUST", "번호": "NO", "주문": "ORDR"},
	)
	src := []byte(`<mxfile host="test"><diagram name="논리" id="p1"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <mxCell id="t1" value="_고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1"/>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1"/>
  <mxCell id="r1d" value="고객번호  int NOT NULL" style="shape=partialRectangle;" vertex="1" parent="r1"/>
  <mxCell id="t2" value="주문고객번호 " style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
</root></mxGraphModel></diagram></mxfile>`)

	once, stats, err := File(src, d)
	if err != nil {
		t.Fatalf("첫 변환: %v", err)
	}
	cells := cellsOf(t, once)
	if got := cells["t1"].Value; got != "_CUST" {
		t.Fatalf("t1.Value = %q; want _CUST (앞 밑줄은 지우지 않는다)", got)
	}
	if got := cells["t2"].Value; got != "ORDR_CUST_NO" {
		t.Fatalf("t2.Value = %q; want ORDR_CUST_NO (뒤 공백은 잘라내고 변환한다)", got)
	}
	// 이름 말고 나머지(타입·NOT NULL·칸 수)는 원문 그대로다.
	if got := cells["r1d"].Value; got != "CUST_NO  int NOT NULL" {
		t.Fatalf("r1d.Value = %q; want 'CUST_NO  int NOT NULL'", got)
	}
	if len(stats.Unmatched) != 0 {
		t.Fatalf("stats.Unmatched = %q; 미매칭이 없어야 한다", stats.Unmatched)
	}

	twice, _, err := File(once, d)
	if err != nil {
		t.Fatalf("두 번째 변환: %v", err)
	}
	if string(once) != string(twice) {
		t.Fatalf("두 번 돌린 결과가 다르다\n1회:\n%s\n2회:\n%s", once, twice)
	}
}

func TestFile_SecondRunUsesLogicalName(t *testing.T) {
	d := fileTestDict(t)
	once, _, err := File([]byte(logicalDrawio), d)
	if err != nil {
		t.Fatalf("첫 변환: %v", err)
	}
	// 사람이 물리명을 손으로 고쳤다고 치자.
	edited := replaceAll(string(once), `label="CUST"`, `label="CUSTOMER"`)

	twice, _, err := File([]byte(edited), d)
	if err != nil {
		t.Fatalf("두 번째 변환: %v", err)
	}
	if got := cellsOf(t, twice)["t1"].Value; got != "CUST" {
		t.Fatalf("t1.Value = %q; logicalName(고객)에서 다시 변환했어야 한다", got)
	}
}

func replaceAll(s, old, new string) string {
	var out string
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

// sameEditDrawio는 두 페이지가 같은 셀 id를 «같은 값»으로 쓴다. 어느 쪽을
// 적용해도 결과가 같으므로 손상이 아니다 — 에러를 내면 안 된다.
const sameEditDrawio = `<mxfile host="test"><diagram name="1페이지" id="p1"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
</root></mxGraphModel></diagram><diagram name="2페이지" id="p2"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
</root></mxGraphModel></diagram></mxfile>`

func dupTestDict(t *testing.T) *glossary.Dict {
	t.Helper()
	return glossary.MustLoadForTest(
		map[string]string{"고객": "CUST", "주문": "ORD"},
		nil,
	)
}

// TestOutputPath_NoExtension은 확장자 없는 입력의 현재 동작을 기록한다.
// erdtool은 .drawio만 다루므로 실전에서는 거의 나오지 않는 경로지만,
// OutputPath 자체는 확장자 유무를 가정하지 않고 동작해야 한다.
func TestOutputPath_NoExtension(t *testing.T) {
	got := OutputPath(filepath.Join("d", "README"), "")
	want := filepath.Join("d", "README.physical")
	if got != want {
		t.Fatalf("OutputPath = %q; want %q", got, want)
	}
}

// TestOutputPath_NameIsEntirelyExtension은 이름 전체가 확장자로 읽히는
// 입력(.drawio, 즉 filepath.Ext가 문자열 전체를 돌려주는 경우)의 현재
// 동작을 기록한다. 줄기(stem)가 빈 문자열이 되어 ".physical.drawio"가
// 나온다 — 어색하지만 규칙(줄기 + ".physical" + 확장자)을 그대로 따른
// 결과이고, 실사용에서 나올 입력이 아니라서 특별 취급하지 않는다.
func TestOutputPath_NameIsEntirelyExtension(t *testing.T) {
	got := OutputPath(filepath.Join("d", ".drawio"), "")
	want := filepath.Join("d", ".physical.drawio")
	if got != want {
		t.Fatalf("OutputPath = %q; want %q", got, want)
	}
}

// TestOutputPath_WithOutDir은 outDir이 주어졌을 때 입력 폴더가 아니라
// outDir 아래에 같은 파일명으로 쓰는지 확인한다.
func TestOutputPath_WithOutDir(t *testing.T) {
	got := OutputPath(filepath.Join("d", "주문.drawio"), "out")
	want := filepath.Join("out", "주문.physical.drawio")
	if got != want {
		t.Fatalf("OutputPath = %q; want %q", got, want)
	}
}

// TestOutputPath_WithoutOutDir은 outDir이 비어 있을 때 입력과 같은 폴더에
// 쓰는지 확인한다.
func TestOutputPath_WithoutOutDir(t *testing.T) {
	got := OutputPath(filepath.Join("d", "주문.drawio"), "")
	want := filepath.Join("d", "주문.physical.drawio")
	if got != want {
		t.Fatalf("OutputPath = %q; want %q", got, want)
	}
}

// TestFile_IdenticalEditOnSharedIDIsNotAnError는 같은 id에 «완전히 같은»
// 편집이 두 페이지에서 겹쳐도 손상으로 취급하지 않는지 확인한다.
//
// 2026-08-31 갱신: 페이지 사이 셀 id 충돌을 거부하던 가드는
// `convert.RewritePlan`이 페이지 단위로 주소되면서
// 없어졌다 — 이제 겹친 id라도 각 페이지가 자기 편집만 받으므로 거부할
// 이유가 없다. 이 테스트는 여전히 유효하다: 편집이 페이지별로 정확히
// 적용되고 결과가 «완전히 같은» 두 편집에서도 안 어긋나는지를 잰다.
func TestFile_IdenticalEditOnSharedIDIsNotAnError(t *testing.T) {
	out, stats, err := File([]byte(sameEditDrawio), dupTestDict(t))
	if err != nil {
		t.Fatalf("같은 편집이 겹쳤을 뿐인데 에러다: %v", err)
	}
	if got := cellsOf(t, out)["t1"].Value; got != "CUST" {
		t.Fatalf("t1.Value = %q; want CUST", got)
	}
	if stats.Total != 2 {
		t.Fatalf("stats = %+v; 페이지 둘을 다 셌어야 한다(Total=2)", stats)
	}
}

// sameIDObjectDrawio는 <object>와 그 안쪽 <mxCell>이 같은 id를 쓰는
// mxfile이다. draw.io는 안쪽 mxCell에 id를 붙이지 않으므로 이런 파일을
// 만들지 않지만, .drawio는 남에게서 받는 입력이다 — 손편집·다른 도구의
// 산출물에서 얼마든지 나온다.
const sameIDObjectDrawio = `<mxfile host="test"><diagram name="논리" id="p1"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <object label="고객" id="t1">
    <mxCell id="t1" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  </object>
</root></mxGraphModel></diagram></mxfile>`

// TestFile_ObjectAndInnerCellShareIDIsNotDoubleWrapped는 «조용히 틀린
// 산출물»을 붙잡는다.
//
// 가드가 없으면 <object> 갈래가 래퍼를 갱신한 뒤 안쪽 <mxCell>이 같은 id로
// 또 걸려 자기만의 <object>로 한 번 더 감싸인다. 이중 래핑된 결과를 다시
// 읽으면 바깥 래퍼에 mxCell 자식이 없어 style도 parent도 빈 셀이 되고,
// 테이블이 파싱 결과에서 통째로 사라진다 — 그런데 Stats는 Converted:1로
// 성공을 보고하고, 그 파일에 generate를 돌리면 테이블 0개가 나온다.
func TestFile_ObjectAndInnerCellShareIDIsNotDoubleWrapped(t *testing.T) {
	out, stats, err := File([]byte(sameIDObjectDrawio), dupTestDict(t))
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if stats.Total != 1 || stats.Converted != 1 {
		t.Fatalf("stats = %+v; want Total=1 Converted=1", stats)
	}

	// 테이블이 살아 있어야 한다 — Stats가 성공이라고 말했으므로.
	ds, err := drawio.LoadDiagramsBytes(out)
	if err != nil {
		t.Fatalf("재파싱: %v", err)
	}
	tables := drawio.FindTables(ds[0].Cells)
	if len(tables) != 1 {
		t.Fatalf("재파싱한 테이블 %d개; 1개여야 한다 — Stats는 변환 성공이라고 했다\n%s",
			len(tables), out)
	}
	if tables[0].Value != "CUST" {
		t.Fatalf("테이블 값 = %q; want CUST\n%s", tables[0].Value, out)
	}
	// 래퍼가 두 겹이 아닌지 직접 본다. 위 검사만으로도 걸리지만, 무엇이
	// 잘못됐는지는 이 검사가 말해준다.
	if n := countOf(string(out), "<object"); n != 1 {
		t.Fatalf("<object>가 %d개다; 1개여야 한다\n%s", n, out)
	}
}

func countOf(s, sub string) int {
	n := 0
	for {
		i := indexOf(s, sub)
		if i < 0 {
			return n
		}
		n++
		s = s[i+len(sub):]
	}
}

// collectLogicalNames는 페이지별로 갈라 돌려준다. 같은 셀 id가 두 페이지에
// 있으면 예전 구조(파일 전체 맵 하나)에서는 뒤 페이지 값이 앞 페이지 값을
// 덮었다 — 그 평탄화가 페이지별 편집이 남의 페이지 논리명을 되살리는
// 사고의 뿌리다.
func TestCollectLogicalNamesSplitsByPage(t *testing.T) {
	const src = `<mxfile host="test">` +
		`<diagram id="pgA" name="A"><mxGraphModel><root>` +
		`<object id="dup" label="CUST" logicalName="고객"><mxCell vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel></diagram>` +
		`<diagram id="pgB" name="B"><mxGraphModel><root>` +
		`<object id="dup" label="ORD" logicalName="주문"><mxCell vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel></diagram></mxfile>`
	got, err := collectLogicalNames([]byte(src))
	if err != nil {
		t.Fatalf("collectLogicalNames: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("페이지 %d개; 2개여야 한다: %v", len(got), got)
	}
	if got[0]["dup"] != "고객" {
		t.Errorf("A페이지 dup = %q; 고객이어야 한다", got[0]["dup"])
	}
	if got[1]["dup"] != "주문" {
		t.Errorf("B페이지 dup = %q; 주문이어야 한다 — 페이지가 평탄화됐다", got[1]["dup"])
	}
}

// 압축된 페이지에서도 논리명을 찾는다. 예전에는 collectLogicalNames가
// 압축 판별·해제를 자기 손으로 했다 — ForEachGraphModel 위로 옮기면서
// 그 갈래를 잃지 않았다는 증거다.
func TestCollectLogicalNamesReadsCompressedPage(t *testing.T) {
	inner := `<mxGraphModel><root>` +
		`<object id="t1" label="CUST" logicalName="고객"><mxCell vertex="1" parent="1"/></object>` +
		`</root></mxGraphModel>`
	enc, err := drawio.Compress(inner) // func Compress(plain string) (string, error)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	src := []byte(`<mxfile host="test"><diagram id="pgA" name="A">` + enc + `</diagram></mxfile>`)

	got, err := collectLogicalNames(src)
	if err != nil {
		t.Fatalf("collectLogicalNames: %v", err)
	}
	if len(got) != 1 || got[0]["t1"] != "고객" {
		t.Errorf("압축된 페이지의 논리명을 못 읽었다: %v", got)
	}
}

// cellsByPage는 cellsOf와 달리 페이지를 안 합친다. 겹친 id를 보려면
// 합치면 안 된다.
func cellsByPage(t *testing.T, data []byte) []map[string]drawio.RawCell {
	t.Helper()
	ds, err := drawio.LoadDiagramsBytes(data)
	if err != nil {
		t.Fatalf("LoadDiagramsBytes: %v", err)
	}
	out := make([]map[string]drawio.RawCell, len(ds))
	for i, d := range ds {
		out[i] = map[string]drawio.RawCell{}
		for _, c := range d.Cells {
			out[i][c.ID] = c
		}
	}
	return out
}

// 두 페이지가 같은 셀 id를 쓰는 파일. 예전에는 File이 하드 에러를 냈다 —
// 편집이 파일 전체에 적용돼 한쪽이 다른 쪽을 덮었기 때문이다. 이제
// 페이지별로 적용되므로 양쪽이 각각 옳게 바뀐다.
const dupIDLogicalDrawio = `<mxfile host="test">` +
	`<diagram name="A" id="pgA"><mxGraphModel><root>` +
	`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
	`<mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>` +
	`</root></mxGraphModel></diagram>` +
	`<diagram name="B" id="pgB"><mxGraphModel><root>` +
	`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
	`<mxCell id="t1" value="번호" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>` +
	`</root></mxGraphModel></diagram></mxfile>`

func TestFile_ConvertsPagesWithCollidingCellIDs(t *testing.T) {
	out, _, err := File([]byte(dupIDLogicalDrawio), fileTestDict(t))
	if err != nil {
		t.Fatalf("File: %v — 겹친 id는 더는 거부 사유가 아니다", err)
	}
	pages := cellsByPage(t, out)
	if len(pages) != 2 {
		t.Fatalf("페이지 %d개; 2개여야 한다", len(pages))
	}
	if got := pages[0]["t1"].Value; got != "CUST" {
		t.Errorf("A페이지 t1 = %q; CUST여야 한다", got)
	}
	if got := pages[1]["t1"].Value; got != "NO" {
		t.Errorf("B페이지 t1 = %q; NO여야 한다 — 한쪽 편집이 다른 쪽을 덮었다", got)
	}
}

// <diagram>에 id가 없는 페이지도 계속 변환된다. 페이지를 첨자로 주소하기로
// 한 이유가 이것이다 — diagram id로 키를 잡았다면 이 페이지가 조용히 안
// 바뀌었을 것이고, 그건 가드를 걷어내려다 같은 종류의 사고를 새로 만드는
// 셈이다.
func TestFile_ConvertsDiagramWithoutID(t *testing.T) {
	const src = `<mxfile host="test"><diagram name="id없음"><mxGraphModel><root>` +
		`<mxCell id="0"/><mxCell id="1" parent="0"/>` +
		`<mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>` +
		`</root></mxGraphModel></diagram></mxfile>`
	out, _, err := File([]byte(src), fileTestDict(t))
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if got := cellsOf(t, out)["t1"].Value; got != "CUST" {
		t.Errorf("t1 = %q; CUST여야 한다 — id 없는 페이지가 변환에서 빠졌다", got)
	}
}
