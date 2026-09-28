package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"erdtool/internal/config"
)

const logicalFixture = `<mxfile host="test"><diagram name="논리" id="p1"><mxGraphModel><root>
  <mxCell id="0"/>
  <mxCell id="1" parent="0"/>
  <mxCell id="t1" value="고객" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1"/>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1"/>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1"/>
  <mxCell id="r1d" value="고객번호 int" style="shape=partialRectangle;" vertex="1" parent="r1"/>
</root></mxGraphModel></diagram></mxfile>`

// writeDict는 pipeline 테스트가 쓸 실제 xlsx 사전을 만든다.
func writeDict(t *testing.T, dir string) string {
	t.Helper()
	// glossary 패키지의 테스트 헬퍼는 비공개이므로 여기서 다시 만든다.
	// 내용은 최소한이면 된다.
	p := filepath.Join(dir, "표준용어사전.xlsx")
	writeMinimalDict(t, p)
	return p
}

// writeMinimalDict는 excelize로 최소한의 표준용어사전 xlsx를 만든다.
// glossary.Load는 시트를 이름이 아니라 순서로 집으므로(첫째=용어사전,
// 둘째=단어사전) 시트 이름 자체는 임의로 둬도 된다.
func writeMinimalDict(t *testing.T, path string) {
	t.Helper()
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })

	fill := func(sheet string, rows [][2]string) {
		if _, err := f.NewSheet(sheet); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
		if err := f.SetCellStr(sheet, "A1", "한글"); err != nil {
			t.Fatalf("SetCellStr: %v", err)
		}
		if err := f.SetCellStr(sheet, "B1", "약어"); err != nil {
			t.Fatalf("SetCellStr: %v", err)
		}
		for i, r := range rows {
			if err := f.SetCellStr(sheet, fmt.Sprintf("A%d", i+2), r[0]); err != nil {
				t.Fatalf("SetCellStr: %v", err)
			}
			if err := f.SetCellStr(sheet, fmt.Sprintf("B%d", i+2), r[1]); err != nil {
				t.Fatalf("SetCellStr: %v", err)
			}
		}
	}
	fill("용어", [][2]string{{"고객번호", "CUST_NO"}})
	fill("단어", [][2]string{{"고객", "CUST"}, {"번호", "NO"}})
	if err := f.DeleteSheet("Sheet1"); err != nil {
		t.Fatalf("DeleteSheet: %v", err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("SaveAs: %v", err)
	}
}

func TestConvert_WritesPhysicalFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "주문.drawio")
	if err := os.WriteFile(src, []byte(logicalFixture), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	dict := writeDict(t, dir)

	stats, outPath, overwritten, err := Convert(src, dict, "")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if want := filepath.Join(dir, "주문.physical.drawio"); outPath != want {
		t.Fatalf("outPath = %q; want %q", outPath, want)
	}
	if overwritten {
		t.Fatal("없던 파일을 새로 만들었는데 overwritten=true다")
	}
	out, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("출력 파일이 없다: %v", err)
	}
	if stats.Total == 0 {
		t.Fatal("변환 시도가 0건이다")
	}
	// Total은 "변환을 시도한 이름 개수"라 시도 횟수지 변경 횟수가 아니다
	// (internal/convert/plan.go의 Stats 주석 참고). dictPath에서 읽은
	// 사전이 실제로 convert.File에 닿았는지는 Converted와 출력 내용을
	// 봐야 알 수 있다 — 빈 사전을 물려도 Total은 그대로라서.
	if stats.Converted == 0 {
		t.Fatal("변경된 이름이 0건이다 — 사전이 실제로 적용됐는지 알 수 없다")
	}
	if !strings.Contains(string(out), "CUST") {
		t.Fatalf("출력에 물리명(CUST)이 없다:\n%s", out)
	}
}

// TestGuardNotOverwriting은 출력이 입력을 덮어쓰지 못하게 막는 계약을
// 직접 검사한다.
//
// 파일명 규칙(X.drawio -> X.physical.drawio) 때문에 정상 경로에서는 두
// 경로가 겹치지 않지만, --out으로 원본 폴더를 그대로 주는 등 겹칠 수 있는
// 길이 있다. 논리 ERD는 사람이 손으로 그린 원본이라 잃으면 되돌릴 길이 없다.
func TestGuardNotOverwriting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "주문.drawio")
	other := filepath.Join(dir, "주문.physical.drawio")

	if err := guardNotOverwriting(src, src); err == nil {
		t.Fatal("입력과 출력이 같으면 에러여야 한다")
	}
	if err := guardNotOverwriting(src, other); err != nil {
		t.Fatalf("다른 경로인데 막았다: %v", err)
	}
	// 같은 파일을 가리키는 다른 표기(./)도 막아야 한다.
	if err := guardNotOverwriting(src, filepath.Join(dir, ".", "주문.drawio")); err == nil {
		t.Fatal("표기만 다른 같은 경로도 막아야 한다")
	}
}

func TestConvertFolder_ConvertsAll(t *testing.T) {
	dir := t.TempDir()
	dict := writeDict(t, dir)
	for _, name := range []string{"주문.drawio", "고객.drawio"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(logicalFixture), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	results, err := ConvertFolder(dir, config.Config{}, dict, "")
	if err != nil {
		t.Fatalf("ConvertFolder: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d; want 2", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.SourceFile, r.Err)
		}
		if r.Stats.Converted == 0 {
			t.Fatalf("%s: 변경된 이름이 0건이다 — 사전이 실제로 적용됐는지 알 수 없다", r.SourceFile)
		}
		out, err := os.ReadFile(r.OutputFile)
		if err != nil {
			t.Fatalf("출력 파일이 없다: %v", err)
		}
		if !strings.Contains(string(out), "CUST") {
			t.Fatalf("%s: 출력에 물리명(CUST)이 없다:\n%s", r.SourceFile, out)
		}
	}
}

// TestConvertFolder_SkipsPhysicalOutputs는 R4 계약을 검사한다: 출력이 입력
// 옆에 놓이므로, 같은 폴더에 두 번 돌리면 1회차 산출물(X.physical.drawio)이
// 2회차 입력이 되어 X.physical.physical.drawio가 생길 수 있다.
// ConvertFolder는 폴더를 읽을 때 그런 파일을 건너뛴다.
func TestConvertFolder_SkipsPhysicalOutputs(t *testing.T) {
	dir := t.TempDir()
	dict := writeDict(t, dir)
	src := filepath.Join(dir, "주문.drawio")
	if err := os.WriteFile(src, []byte(logicalFixture), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, _, _, err := Convert(src, dict, ""); err != nil {
		t.Fatalf("1회차 Convert: %v", err)
	}

	results, err := ConvertFolder(dir, config.Config{}, dict, "")
	if err != nil {
		t.Fatalf("ConvertFolder: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d; want 1 (physical 산출물은 건너뛰어야 한다): %+v", len(results), results)
	}
	if results[0].SourceFile != src {
		t.Fatalf("SourceFile = %q; want %q", results[0].SourceFile, src)
	}
	for _, name := range []string{"주문.physical.physical.drawio"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Fatalf("%s가 만들어지면 안 된다", name)
		}
	}
}

// TestConvertFolder_OutDirCollisionAcrossSubdirsIsAnError는 I1을 검사한다.
//
// convert.OutputPath는 outDir이 주어지면 원본이 어느 폴더에 있었는지는
// 버리고 basename만 본다(TestOutputPath_WithOutDir 참고). 재귀 스캔에서
// 서로 다른 하위 폴더에 같은 이름의 파일이 있으면(a/주문.drawio,
// b/주문.drawio) 둘 다 같은 출력 경로로 몰린다. 가드가 없으면 둘째 쓰기가
// 첫째를 조용히 덮어쓰고도 두 결과 모두 Err == nil로 돌아간다 — 사용자는
// 두 파일이 변환됐다고 듣지만 하나는 사라진 뒤다. 하나는 성공, 하나는
// 그 사실을 설명하는 에러로 나와야 한다.
func TestConvertFolder_OutDirCollisionAcrossSubdirsIsAnError(t *testing.T) {
	dir := t.TempDir()
	dict := writeDict(t, dir)
	for _, sub := range []string{"a", "b"} {
		subDir := filepath.Join(dir, sub)
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "주문.drawio"), []byte(logicalFixture), 0o644); err != nil {
			t.Fatalf("write %s: %v", sub, err)
		}
	}
	outDir := filepath.Join(dir, "out")

	results, err := ConvertFolder(dir, config.Config{Recursive: true}, dict, outDir)
	if err != nil {
		t.Fatalf("ConvertFolder: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d; want 2: %+v", len(results), results)
	}

	var succeeded, failed int
	var winner string // 실제로 파일을 쓴 쪽의 SourceFile
	wantOut := filepath.Join(outDir, "주문.physical.drawio")
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		succeeded++
		winner = r.SourceFile
		if r.OutputFile != wantOut {
			t.Fatalf("OutputFile = %q; want %q", r.OutputFile, wantOut)
		}
		if _, statErr := os.Stat(r.OutputFile); statErr != nil {
			t.Fatalf("출력 파일이 없다: %v", statErr)
		}
	}
	for _, r := range results {
		if r.Err == nil {
			continue
		}
		failed++
		// 에러가 어느 입력과 충돌했는지 사람이 알 수 있어야 한다. 메시지가
		// %q로 경로를 찍으므로(백슬래시가 이스케이프된다) 기대값도 같은
		// 방식으로 인용해 비교한다.
		msg := r.Err.Error()
		if !strings.Contains(msg, quotedInner(wantOut)) {
			t.Fatalf("에러에 충돌한 출력 경로가 없다: %v", r.Err)
		}
		if !strings.Contains(msg, quotedInner(winner)) {
			t.Fatalf("에러에 먼저 쓴 입력 파일(%q)이 없다: %v", winner, r.Err)
		}
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("succeeded=%d failed=%d; want 1/1: %+v", succeeded, failed, results)
	}
}

// quotedInner는 %q로 문자열을 인용했을 때 감싸는 큰따옴표를 뺀 안쪽
// 내용이다. 에러 메시지가 %q로 경로를 찍어 백슬래시가 이스케이프되므로,
// 테스트에서 원본 경로를 그대로 부분 문자열로 찾으면 어긋난다.
func quotedInner(s string) string {
	q := fmt.Sprintf("%q", s)
	return q[1 : len(q)-1]
}

// TestConvert_SignalsOverwrite는 «이미 있던 물리 파일을 대체했다»는 사실이
// 호출자에게 전달되는지 본다.
//
// 스펙 결정 5는 convert를 별도 서브커맨드로 둔 이유를 "중간 산출물(물리
// ERD)을 사람이 검수·수정한 다음 리포트를 만들 수 있어야" 한다고 했다.
// 재실행이 그 손댄 파일을 아무 신호 없이 덮어쓰면 그 흐름이 조용히 깨진다.
// logicalName은 이름만 지켜준다 — 레이아웃·손으로 더한 컬럼·메모는 아니다.
func TestConvert_SignalsOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "주문.drawio")
	if err := os.WriteFile(src, []byte(logicalFixture), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	dict := writeDict(t, dir)

	_, dst, overwritten, err := Convert(src, dict, "")
	if err != nil {
		t.Fatalf("1회차 Convert: %v", err)
	}
	if overwritten {
		t.Fatal("1회차는 새로 만든 것이다 — overwritten=false여야 한다")
	}

	// 사람이 물리 파일을 손으로 고쳤다고 치자.
	if err := os.WriteFile(dst, []byte("사람이 손댄 내용"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, overwritten, err = Convert(src, dict, "")
	if err != nil {
		t.Fatalf("2회차 Convert: %v", err)
	}
	if !overwritten {
		// --force로 막지 않는 대신(멱등성이 설계 목표다) 반드시 알려야 한다.
		t.Fatal("이미 있던 파일을 대체했는데 overwritten=false다 — 사용자는 자기가 고친 파일이 사라진 줄 모른다")
	}
}

// TestConvertFolder_SignalsOverwrite는 같은 신호가 폴더 경로에서도 나오는지
// 본다. 단일 파일과 폴더가 같은 사실을 다르게 보고하면 안 된다(R8).
func TestConvertFolder_SignalsOverwrite(t *testing.T) {
	dir := t.TempDir()
	dict := writeDict(t, dir)
	src := filepath.Join(dir, "주문.drawio")
	if err := os.WriteFile(src, []byte(logicalFixture), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	first, err := ConvertFolder(dir, config.Config{}, dict, "")
	if err != nil {
		t.Fatalf("1회차 ConvertFolder: %v", err)
	}
	if len(first) != 1 || first[0].Err != nil {
		t.Fatalf("1회차 결과가 이상하다: %+v", first)
	}
	if first[0].Overwritten {
		t.Fatal("1회차는 새로 만든 것이다 — Overwritten=false여야 한다")
	}

	// 2회차: ConvertFolder는 *.physical.drawio를 입력에서 건너뛰지만(R4)
	// 원본 주문.drawio는 그대로 다시 변환하므로 산출물을 대체한다.
	second, err := ConvertFolder(dir, config.Config{}, dict, "")
	if err != nil {
		t.Fatalf("2회차 ConvertFolder: %v", err)
	}
	if len(second) != 1 || second[0].Err != nil {
		t.Fatalf("2회차 결과가 이상하다: %+v", second)
	}
	if !second[0].Overwritten {
		t.Fatal("2회차는 1회차 산출물을 대체했는데 Overwritten=false다")
	}
}

// ConvertStats는 «쓰지 않고» 통계만 낸다. convert --dry-run이 이걸 탄다.
//
// 예전에는 cmd/erdtool이 os.ReadFile + glossary.Load + convert.File을 직접
// 다시 조립했다. 그러면 dry-run이 보고하는 통계가 실제 실행의 것과 갈릴
// 수 있는데, 갈렸는지 볼 방법이 없었다 — dry-run 경로가 pipeline 밖에
// 있어서 pipeline 테스트가 닿지 못했다. 여기서 둘을 나란히 놓고 본다.
func TestConvertStats_MatchesConvertAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "논리.drawio")
	if err := os.WriteFile(src, []byte(logicalFixture), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	dict := writeDict(t, dir)

	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	dryStats, err := ConvertStats(src, dict)
	if err != nil {
		t.Fatalf("ConvertStats: %v", err)
	}

	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("파일이 %d개에서 %d개가 됐다 — dry-run은 아무것도 쓰면 안 된다",
			len(before), len(after))
	}

	realStats, _, _, err := Convert(src, dict, "")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if dryStats.Total != realStats.Total || dryStats.Converted != realStats.Converted {
		t.Errorf("통계가 갈렸다: dry=%+v real=%+v", dryStats, realStats)
	}
	if !reflect.DeepEqual(dryStats.Unmatched, realStats.Unmatched) {
		t.Errorf("미매칭 목록이 갈렸다: dry=%v real=%v", dryStats.Unmatched, realStats.Unmatched)
	}
	if !reflect.DeepEqual(dryStats.Details, realStats.Details) {
		t.Errorf("상세가 갈렸다:\ndry=%+v\nreal=%+v", dryStats.Details, realStats.Details)
	}
}

// 읽기 실패도 실제 실행과 같은 문구여야 한다 — 어느 파일이 문제인지가
// 반드시 들어간다. 여러 파일을 시도해 본 사용자에게 "EOF"만 찍히면
// 어느 것이 문제인지 모른다.
func TestConvertStats_ReadErrorNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	dict := writeDict(t, dir)
	missing := filepath.Join(dir, "없다.drawio")

	_, err := ConvertStats(missing, dict)
	if err == nil {
		t.Fatal("없는 파일이 통과했다")
	}
	if !strings.Contains(err.Error(), "없다.drawio") {
		t.Errorf("문구에 파일 이름이 없다: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "read ") {
		t.Errorf("문구=%q; 실제 실행과 같은 \"read %%q: ...\" 꼴이어야 한다", err)
	}
}
