package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"erdtool/internal/config"
	"erdtool/internal/convert"
	"erdtool/internal/glossary"
	"erdtool/internal/pipeline"
)

func TestPrintFolderResults_SignalsFailure(t *testing.T) {
	// 실측: 폴더 일괄 처리에서 파일 하나가 [SKIP]으로 빠져도 종료 코드가
	// 0이었다. watch는 한 번이라도 실패하면 1로 끝나는데 generate만 달라서,
	// 스크립트/CI로 폴더를 돌리면 실패를 놓친다.
	results := []pipeline.FileResult{
		{SourceFile: "ok.drawio", Result: pipeline.Result{OutputDir: "ok_report", TableCount: 2}},
		{SourceFile: "broken.drawio", Err: errors.New("parse mxfile: EOF")},
	}
	var out, errOut bytes.Buffer

	if failed := printFolderResults(&out, &errOut, results); !failed {
		t.Fatal("파일 하나가 실패했으면 failed=true여야 한다")
	}
	if !strings.Contains(out.String(), "[OK] ok.drawio -> ok_report (2 tables)") {
		t.Fatalf("성공한 파일은 stdout에 찍혀야 한다: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "[SKIP] broken.drawio: parse mxfile: EOF") {
		t.Fatalf("실패한 파일은 stderr에 찍혀야 한다: %q", errOut.String())
	}
}

func TestPrintFolderResults_AllGood(t *testing.T) {
	results := []pipeline.FileResult{
		{SourceFile: "a.drawio", Result: pipeline.Result{OutputDir: "a_report", TableCount: 1}},
	}
	var out, errOut bytes.Buffer

	if failed := printFolderResults(&out, &errOut, results); failed {
		t.Fatal("전부 성공했으면 failed=false여야 한다")
	}
	if errOut.Len() != 0 {
		t.Fatalf("실패가 없으면 stderr는 비어야 한다: %q", errOut.String())
	}
}

func TestResolveConfig_ExplicitBeatsDiscovery(t *testing.T) {
	// --config를 명시했으면 자동탐색은 아예 하지 않는다. 옆에 놓인
	// erdtool.yaml이 조용히 끼어들면 "왜 내가 준 설정이 안 먹지"가 된다.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "erdtool.yaml"), []byte("page_as_domain: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	explicit := filepath.Join(dir, "mine.yaml")
	if err := os.WriteFile(explicit, []byte("page_as_domain: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, from, err := resolveConfig(explicit, dir)
	if err != nil {
		t.Fatal(err)
	}
	if from != explicit {
		t.Fatalf("명시한 설정이 쓰여야 한다: %q", from)
	}
	if cfg.PageAsDomain {
		t.Fatal("자동탐색 쪽 값이 섞였다")
	}
}

func TestResolveConfig_DiscoversBesideTarget(t *testing.T) {
	dir := t.TempDir()
	auto := filepath.Join(dir, "erdtool.yaml")
	if err := os.WriteFile(auto, []byte("page_as_domain: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, from, err := resolveConfig("", dir)
	if err != nil {
		t.Fatal(err)
	}
	if from != auto {
		t.Fatalf("자동탐색이 안 됐다: %q", from)
	}
	if !cfg.PageAsDomain {
		t.Fatal("자동탐색한 설정 내용이 반영되지 않았다")
	}
}

func TestResolveConfig_NoneIsNotAnError(t *testing.T) {
	cfg, from, err := resolveConfig("", t.TempDir())
	if err != nil {
		t.Fatalf("설정이 없는 것은 에러가 아니다: %v", err)
	}
	if from != "" {
		t.Fatalf("쓰인 설정이 없어야 한다: %q", from)
	}
	if cfg.PageAsDomain {
		t.Fatal("기본값이어야 한다")
	}
}

func TestResolveDictionary_ExplicitBeatsAll(t *testing.T) {
	// --dictionary를 명시했으면 설정 파일도 자동탐색도 아예 보지 않는다.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "표준용어사전.xlsx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Dictionary: "cfg-dict.xlsx"}

	got, auto, err := resolveDictionary("explicit.xlsx", cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "explicit.xlsx" {
		t.Fatalf("명시한 사전이 쓰여야 한다: %q", got)
	}
	if auto {
		t.Fatal("명시했으면 auto=false여야 한다")
	}
}

func TestResolveDictionary_ConfigBeatsDiscovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "표준용어사전.xlsx"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Dictionary: "cfg-dict.xlsx"}

	got, auto, err := resolveDictionary("", cfg, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "cfg-dict.xlsx" {
		t.Fatalf("설정 파일의 사전이 쓰여야 한다: %q", got)
	}
	if auto {
		t.Fatal("설정에서 왔으면 auto=false여야 한다")
	}
}

func TestResolveDictionary_DiscoversBesideTarget(t *testing.T) {
	dir := t.TempDir()
	dictPath := filepath.Join(dir, "표준용어사전.xlsx")
	if err := os.WriteFile(dictPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, auto, err := resolveDictionary("", config.Config{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dictPath {
		t.Fatalf("자동탐색이 안 됐다: %q", got)
	}
	if !auto {
		t.Fatal("자동탐색으로 찾았으면 auto=true여야 한다")
	}
}

func TestResolveDictionary_NoneIsError(t *testing.T) {
	// 사전 없이 돌리면 전부 미매칭이라 원본 복사본이 나오는데, 그걸 조용히
	// 성공으로 보고하면 안 된다.
	_, _, err := resolveDictionary("", config.Config{}, t.TempDir())
	if err == nil {
		t.Fatal("사전이 전혀 없으면 에러여야 한다")
	}
	// 이 문구가 곧 처방이다 — 사전을 주는 세 가지 길을 사용자가 배우는
	// 자리는 여기뿐이고, 스펙도 이 실패를 따로 짚는다("셋 다 없으면 에러로
	// 멈춘다"). err != nil만 보면 문구가 "no dictionary"로 쪼그라들어도
	// 테스트는 통과하고, 사용자는 무엇을 해야 하는지 모른 채 남는다.
	for _, remedy := range []string{"--dictionary", "dictionary:", "표준용어사전.xlsx"} {
		if !strings.Contains(err.Error(), remedy) {
			t.Fatalf("에러 문구에 %q가 없다 — 사전을 주는 세 가지 길을 다 말해야 한다: %v", remedy, err)
		}
	}
}

// assertStatsIsProducible은 손으로 적은 Stats가 «엔진이 만들 수 있는 모양»인지
// 검사한다. printDryRun은 Stats를 통째로 받는 순수 포매터라 숫자와 Details가
// 서로 맞는지 아무도 보지 않는다 — 그래서 픽스처가 조용히 어긋나도 테스트는
// 계속 통과한다. 실제로 그렇게 어긋난 적이 있다.
//
// 검사하는 것은 buildEdits(internal/convert/plan.go)가 «Details로부터 반드시
// 따라 나오게» 만드는 두 가지뿐이다:
//
//	Total     == len(Details)   — record()가 둘을 한 번에 늘린다(plan.go:52-58)
//	Unmatched == 미매칭 Part.Source를 등장 순으로 중복 제거(plan.go:60-72)
//
// Converted는 «검사하지 않는다». 그 값은 새 값과 «셀의 현재 값»을 비교해
// 나오는데(plan.go:82, :107) Detail에는 현재 값이 없다. 2회차 실행에서는
// 현재 값이 이미 물리명이라 Logical과도 다르므로, Detail만 보고 유도하면
// 틀린 답을 «검증»하게 된다. 유도할 수 없는 것을 유도하는 척하느니 손으로
// 확인하고 주석에 근거를 남기는 편이 낫다.
func assertStatsIsProducible(t *testing.T, s convert.Stats) {
	t.Helper()
	if s.Total != len(s.Details) {
		t.Fatalf("Total = %d; Details가 %d행이다. buildEdits는 둘을 한 번에 늘린다", s.Total, len(s.Details))
	}
	var want []string
	seen := map[string]bool{}
	for _, d := range s.Details {
		for _, p := range d.Parts {
			if p.Matched || strings.TrimSpace(p.Source) == "" || seen[p.Source] {
				continue
			}
			seen[p.Source] = true
			want = append(want, p.Source)
		}
	}
	if !slices.Equal(s.Unmatched, want) {
		t.Fatalf("Unmatched = %v; Details에서 유도하면 %v다", s.Unmatched, want)
	}
	if s.Converted > s.Total {
		t.Fatalf("Converted = %d > Total = %d", s.Converted, s.Total)
	}
}

func TestPrintDryRun(t *testing.T) {
	// 이 Stats는 손으로 적었지만 «엔진이 실제로 만드는 모양»이다. 실물
	// 사전과 buildEdits로 확인한 값이다(테이블 «고객», 컬럼 «약어명», «약어»):
	//
	//	Total=3  Converted=2  Unmatched=[약어]
	//
	// 아래 assertStatsIsProducible이 그중 «Details에서 유도되는» 부분을
	// 매번 다시 검사한다 — 이 픽스처는 네 번 편집되는 동안 매번 «엔진이
	// 만드는 모양»이라고 주장했지만 실제로는 어긋난 적이 있다.
	stats := convert.Stats{
		Total:     3,
		Converted: 2, // 고객->CUST, 약어명->약어_NM (약어->약어는 값이 안 바뀐다)
		Unmatched: []string{"약어"},
		Details: []convert.Detail{
			// 단어사전에 통째로 있는 이름은 [분해]가 아니다 — 분해가 아니라
			// 조회로 풀렸다. Convert가 실제로 이 조합을 만든다.
			{Kind: "table", Logical: "고객", Physical: "CUST", Route: glossary.RouteWordWhole,
				Parts: []glossary.Part{{Source: "고객", Abbr: "CUST", Matched: true}}},
			// 붙어 있는 미매칭 글자는 한 조각이다("약", "어"가 아니라 "약어").
			// 이 표는 실물 Convert의 결과를 찍는 자리이므로, 엔진이 만들 수
			// 없는 모양을 여기 적어두면 표가 무엇을 보여주는지 오해하게 된다.
			{Kind: "column", Logical: "약어명", Physical: "약어_NM", Route: glossary.RouteSegmented,
				Parts: []glossary.Part{
					{Source: "약어", Abbr: "약어", Matched: false},
					{Source: "명", Abbr: "NM", Matched: true},
				}},
			// 아무것도 못 맞혀 이름이 통째로 그대로인 줄은 [분해]가 아니라
			// [미변환]이다. 분해가 돌기는 했어도 답에 기여한 것이 없으므로
			// «추측이 섞였다»고 경고할 것이 없다.
			//
			// 예시가 «정보»였는데 그건 실물 사전에서 안 나오는 조합이다 —
			// 정보는 단어사전에 있어서 INFO [단어사전 통째]가 된다. 실물로
			// 확인한 «약어»로 바꿨다(어느 사전에도 없고 분해도 못 맞힌다).
			// 그때 Unmatched를 함께 안 고쳐서, 표에 없는 «정보»를 요약 줄이
			// 미매칭으로 세는 상태가 한 커밋 살았다. 그 부류를 막는 것이
			// assertStatsIsProducible이다.
			{Kind: "column", Logical: "약어", Physical: "약어", Route: glossary.RouteUnmatched,
				Parts: []glossary.Part{{Source: "약어", Abbr: "약어", Matched: false}}},
		},
	}

	assertStatsIsProducible(t, stats)

	var buf bytes.Buffer
	printDryRun(&buf, stats)
	out := buf.String()

	// F3: substring 검사는 "미매칭"이 줄별 절에 있든 요약 절에 있든 다
	// 통과시키고, 요약의 단위(예: "N개" vs "N건")가 틀려도 잡지 못한다.
	// 표 전체를 손으로 계산한 문자열과 정확 비교한다 — %-6s 열 정렬,
	// [분해] 경로 표시, 미매칭 조각 나열, 요약 줄의 단위까지 전부 고정한다.
	want := `  table  고객 -> CUST  [단어사전 통째]
  column 약어명 -> 약어_NM  [분해]  미매칭: 약어
  column 약어 -> 약어  [미변환]  미매칭: 약어
요약: 3건 중 2건 변환, 미매칭 낱말 1개(약어)
`
	if out != want {
		t.Fatalf("printDryRun 출력이 다르다:\n got: %q\nwant: %q", out, want)
	}
}

func TestPrintConvertResults_SignalsFailure(t *testing.T) {
	// printFolderResults를 그대로 쓰면 outDir/tableCount가 없어 "(0 tables)"
	// 같은 거짓 문구가 나온다(R1) — convert 전용 출력 함수를 검증한다.
	results := []pipeline.ConvertFileResult{
		{SourceFile: "ok.drawio", OutputFile: "ok.physical.drawio",
			Stats: convert.Stats{Total: 3, Converted: 2, Unmatched: []string{"약"}}},
		{SourceFile: "broken.drawio", Err: errors.New("parse mxfile: EOF")},
	}
	var out, errOut bytes.Buffer

	if failed := printConvertResults(&out, &errOut, "somedir", results); !failed {
		t.Fatal("파일 하나가 실패했으면 failed=true여야 한다")
	}
	// F1: Unmatched는 «낱말» 목록이지 «이름» 개수가 아니다. 앞 절의 "건"을
	// 그대로 상속하는 "1개 미매칭"이 아니라 명사가 붙은 "미매칭 낱말 1개"여야
	// "22건 중 22건 변환, 1건이 미매칭으로 실패했나?"로 잘못 읽히지 않는다.
	if !strings.Contains(out.String(), "[CONVERTED] ok.physical.drawio (3건 중 2건 변환, 미매칭 낱말 1개)") {
		t.Fatalf("성공한 파일은 stdout에 찍혀야 한다: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "[SKIP] broken.drawio: parse mxfile: EOF") {
		t.Fatalf("실패한 파일은 stderr에 찍혀야 한다: %q", errOut.String())
	}
}

func TestPrintConvertResults_AllGood(t *testing.T) {
	results := []pipeline.ConvertFileResult{
		{SourceFile: "a.drawio", OutputFile: "a.physical.drawio",
			Stats: convert.Stats{Total: 1, Converted: 1}},
	}
	var out, errOut bytes.Buffer

	if failed := printConvertResults(&out, &errOut, "somedir", results); failed {
		t.Fatal("전부 성공했으면 failed=false여야 한다")
	}
	if !strings.Contains(out.String(), "[CONVERTED] a.physical.drawio (1건 중 1건 변환)") {
		t.Fatalf("미매칭이 0이면 그 절을 빼야 한다: %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("실패가 없으면 stderr는 비어야 한다: %q", errOut.String())
	}
}

func TestPrintConvertResults_EmptyResultsReportsAndSucceeds(t *testing.T) {
	// F4: ConvertFolder는 이미 변환된 산출물(*.physical.drawio)을 건너뛴다
	// (R4). 같은 폴더에 두 번째로 erdtool convert를 돌리면 results가 통째로
	// 비어 돌아온다 — 아무것도 안 찍고 exit 0으로 끝나면 스크립트에게는
	// 성공과 구별되지 않고 사람에게는 그냥 의아하다.
	var out, errOut bytes.Buffer

	if failed := printConvertResults(&out, &errOut, "already_done", nil); failed {
		t.Fatal("빈 결과는 실패가 아니다 — failed=false여야 한다")
	}
	if !strings.Contains(out.String(), "already_done") {
		t.Fatalf("어느 대상에서 아무것도 변환하지 않았는지 찍어야 한다: %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("빈 결과는 에러가 아니므로 stderr는 비어야 한다: %q", errOut.String())
	}
}

// TestPrintConvertResults_OverwrittenGetsItsOwnWord는 «만들었다»와 «당신이
// 고친 파일을 덮어썼다»가 구별되는지 본다.
//
// 스펙 결정 5는 물리 ERD를 사람이 검수·수정한 다음 리포트를 만드는 흐름을
// 전제한다. convert 재실행은 그 손댄 파일을 대체하는데, logicalName이
// 지켜주는 것은 이름뿐이다(레이아웃·손으로 더한 컬럼·메모는 아니다).
// 재실행 자체를 막지는 않는다 — 멱등성이 설계 목표라 두 번째 실행을
// 실패시키면 그 목표와 싸운다. 대신 매 실행마다 말한다.
func TestPrintConvertResults_OverwrittenGetsItsOwnWord(t *testing.T) {
	results := []pipeline.ConvertFileResult{
		{SourceFile: "new.drawio", OutputFile: "new.physical.drawio",
			Stats: convert.Stats{Total: 1, Converted: 1}},
		{SourceFile: "old.drawio", OutputFile: "old.physical.drawio",
			Stats: convert.Stats{Total: 1, Converted: 1}, Overwritten: true},
	}
	var out, errOut bytes.Buffer

	if failed := printConvertResults(&out, &errOut, "somedir", results); failed {
		t.Fatal("덮어쓰기는 실패가 아니다 — failed=false여야 한다")
	}
	want := "[CONVERTED] new.physical.drawio (1건 중 1건 변환)\n" +
		"[OVERWRITTEN] old.physical.drawio (1건 중 1건 변환)\n"
	if out.String() != want {
		t.Fatalf("출력이 다르다:\n got: %q\nwant: %q", out.String(), want)
	}
	if errOut.Len() != 0 {
		t.Fatalf("덮어쓰기는 에러가 아니므로 stderr는 비어야 한다: %q", errOut.String())
	}
}

// TestCheckConvertFlags는 «사용자가 친 것을 조용히 무시하지 않는다»를
// 검사한다. 무시된 플래그는 사용자에게 거짓 사실을 믿게 만든다 —
// --recursive를 걸었는데 아무 일도 없었으면 "하위 폴더에 변환할 것이
// 없었구나"로 읽히고, --out을 걸었는데 파일이 없으면 지정한 폴더를
// 뒤지게 된다.
func TestCheckConvertFlags(t *testing.T) {
	cases := []struct {
		name          string
		isDir         bool
		recursive     bool
		dryRun        bool
		outDir        string
		wantErrSubstr string
	}{
		{name: "폴더+dry-run", isDir: true, dryRun: true, wantErrSubstr: "--dry-run"},
		{name: "파일+recursive", isDir: false, recursive: true, wantErrSubstr: "--recursive"},
		{name: "dry-run+out", isDir: false, dryRun: true, outDir: "out", wantErrSubstr: "--out"},
		{name: "폴더+recursive", isDir: true, recursive: true},
		{name: "파일+out", isDir: false, outDir: "out"},
		{name: "파일+dry-run", isDir: false, dryRun: true},
		{name: "플래그 없음"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkConvertFlags(c.isDir, c.recursive, c.dryRun, c.outDir)
			if c.wantErrSubstr == "" {
				if err != nil {
					t.Fatalf("정상 조합인데 막았다: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("조용히 무시하면 안 되는 조합인데 에러가 없다 (%+v)", c)
			}
			// 어느 플래그가 문제인지 문구에 있어야 사용자가 무엇을 지울지 안다.
			if !strings.Contains(err.Error(), c.wantErrSubstr) {
				t.Fatalf("에러에 %q가 없다: %v", c.wantErrSubstr, err)
			}
		})
	}
}

func TestFormatReverseResult(t *testing.T) {
	res := pipeline.ReverseResult{
		Target: "postgres://user:***@localhost/mydb", OutputPath: "mydb.drawio",
		Pages: 2, Tables: 14, Relations: 18,
	}
	got := formatReverseResult(res)
	want := "[REVERSED] postgres://user:***@localhost/mydb -> mydb.drawio (2개 페이지, 14개 테이블, 관계 18건)\n"
	if got != want {
		t.Errorf("결과 줄 = %q; want %q", got, want)
	}
}

// 덮어썼으면 그렇게 말해야 한다. convert가 [CONVERTED]/[OVERWRITTEN]을
// 가르는 것과 같은 이유 — 사용자가 무엇이 없어졌는지 알아야 한다.
func TestFormatReverseResult_OverwrittenGetsItsOwnWord(t *testing.T) {
	got := formatReverseResult(pipeline.ReverseResult{
		Target: "./app.db", OutputPath: "app.drawio", Overwritten: true,
		Pages: 1, Tables: 2, Relations: 1,
	})
	if !strings.HasPrefix(got, "[OVERWRITTEN] ") {
		t.Errorf("결과 줄 = %q; 덮어썼으면 그렇게 말해야 한다", got)
	}
}

// 페이지를 가로지르는 관계는 개수만이 아니라 «이름까지» 찍어야 한다.
// 조용히 버리면 산출물이 관계가 원래 없었다고 거짓말하게 된다.
func TestFormatReverseResult_ReportsCrossPageRelationsByName(t *testing.T) {
	got := formatReverseResult(pipeline.ReverseResult{
		Target: "pg", OutputPath: "x.drawio", Pages: 2, Tables: 4, Relations: 3,
		SkippedCross: []string{"sales.orders.customer_id -> hr.employee.id"},
	})
	if !strings.Contains(got, "sales.orders.customer_id -> hr.employee.id") {
		t.Errorf("결과 = %q; 그리지 못한 관계의 이름이 없다", got)
	}
	if !strings.Contains(got, "1건") {
		t.Errorf("결과 = %q; 그리지 못한 개수가 없다", got)
	}
}

func TestCheckAnnotateFlags(t *testing.T) {
	cases := []struct {
		name                     string
		isDir, recursive, dryRun bool
		wantErr                  bool
	}{
		{name: "파일 기본", wantErr: false},
		{name: "폴더 재귀", isDir: true, recursive: true, wantErr: false},
		{name: "파일에 재귀", recursive: true, wantErr: true},
		{name: "폴더에 dry-run", isDir: true, dryRun: true, wantErr: true},
		{name: "파일에 dry-run은 허용", dryRun: true, wantErr: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkAnnotateFlags(c.isDir, c.recursive, c.dryRun)
			if (err != nil) != c.wantErr {
				t.Errorf("err=%v; wantErr=%v", err, c.wantErr)
			}
		})
	}
}

// --clean --dry-run은 checkAnnotateFlags를 통과하는 조합이지만(허용된
// 조합이다), 화면에 찍을 문구는 다섯 갈래([FAILED]/[UNCHANGED]/[CLEANED]/
// [DRY-RUN]/[ANNOTATED]) 중 어느 것을 고르느냐가 분기 순서에 달렸다.
// 특히 clean && dryRun을 clean보다 먼저 걸지 않으면 파일이 그대로인데도
// "표식을 지웠다"고 말해 버린다 — 도구가 한 일과 다른 말을 하는 것을
// 이 테스트가 고정한다.
//
// Marked와 Pages는 서로 다른 값(3과 1)을 쓴다 — 둘 다 1이면 포맷 문자열의
// 두 인자 순서가 뒤바뀌어도(마크 개수 자리에 페이지 개수가 들어가도) 이
// 테스트가 못 잡는다.
func TestAnnotateResultLine(t *testing.T) {
	cases := []struct {
		name        string
		r           pipeline.AnnotateResult
		clean       bool
		dryRun      bool
		wantPrefix  string
		wantSubstr  []string
		wantNoParts []string
	}{
		{
			name:       "일반 annotate",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Marked: 3, Pages: 1, Findings: 2, Changed: true},
			wantPrefix: "[ANNOTATED] ",
			wantSubstr: []string{"셀 3개 마크", "페이지 1개 요약", "진단 2건"},
		},
		{
			name:       "바뀐 것 없음",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Findings: 0, Changed: false},
			wantPrefix: "[UNCHANGED] ",
			wantSubstr: []string{"진단 0건"},
		},
		{
			// clean이면 pipeline.Annotate가 검증을 아예 안 돈다
			// (internal/pipeline/annotate.go: clean 분기는 ir.Assemble도
			// validate.Required/Optional도 안 부른다) — 그래서 clean
			// 경로의 r.Findings는 항상 0이다. 이걸 일반 !r.Changed
			// 갈래로 보내면 "(진단 0건)"이라고 찍혀 "이 도면엔 고칠 게
			// 없다"는 뜻으로 읽히는데, 실제로는 검증 자체를 안 했다
			// — 지울 표식이 없었을 뿐이다. 이 케이스가 그 구분을 고정한다.
			name:        "clean인데 지울 표식이 없음: 검증 결과인 척하면 안 된다",
			r:           pipeline.AnnotateResult{SourceFile: "a.drawio", Findings: 0, Changed: false},
			clean:       true,
			wantPrefix:  "[UNCHANGED] ",
			wantSubstr:  []string{"지울 표식 없음"},
			wantNoParts: []string{"진단"},
		},
		{
			name:       "실패",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Err: errors.New("boom")},
			wantPrefix: "[FAILED] ",
			wantSubstr: []string{"boom"},
		},
		{
			name:       "dry-run (annotate만)",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Marked: 3, Pages: 1, Findings: 2, Changed: true},
			dryRun:     true,
			wantPrefix: "[DRY-RUN] ",
			wantSubstr: []string{"셀 3개 마크", "페이지 1개 요약", "진단 2건", "파일은 안 바꿈"},
		},
		{
			name:       "clean (실제로 지움)",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Marked: 1, Pages: 1, Changed: true},
			clean:      true,
			wantPrefix: "[CLEANED] ",
			wantSubstr: []string{"표식 1개", "요약 박스 1개"},
		},
		{
			// 앵커 없는 진단(duplicate_table_name, 파싱 안 된 행의
			// shape_violation)만 나온 페이지는 마크된 셀이
			// 하나도 없이 요약 박스만 받는다. 그 파일에 --clean을 돌리면
			// Marked=0인데 파일은 실제로 바뀐다(박스가 사라진다) — Marked만
			// 찍으면 "0개 제거"라고 말하면서 사용자 파일의 sha256을 바꾸는
			// 셈이다. Result.Pages가 이 사실을 이미 들고 있으니 찍는다.
			name:       "clean: 마크는 없고 요약 박스만 지운 파일",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Marked: 0, Pages: 1, Changed: true},
			clean:      true,
			wantPrefix: "[CLEANED] ",
			wantSubstr: []string{"표식 0개", "요약 박스 1개"},
		},
		{
			// 이 케이스가 이번 수정의 핵심이다: 파일은 안 바뀌었으니
			// [CLEANED]로 "지웠다"고 말하면 안 되고, [DRY-RUN]으로
			// "지울 것"만 예고해야 한다.
			name:        "clean + dry-run: 지울 것만 찍고 지웠다고 말하지 않는다",
			r:           pipeline.AnnotateResult{SourceFile: "a.drawio", Marked: 1, Pages: 1, Changed: true},
			clean:       true,
			dryRun:      true,
			wantPrefix:  "[DRY-RUN] ",
			wantSubstr:  []string{"표식 1개", "요약 박스 1개", "제거 예정", "파일은 안 바꿈"},
			wantNoParts: []string{"[CLEANED]"},
		},
		{
			// clean && dryRun 갈래도 같은 사실을 예고해야 한다 — 미리보기가
			// 실제 실행과 다른 숫자를 말하면 미리보기의 뜻이 없다.
			name:       "clean + dry-run: 요약 박스만 지울 파일",
			r:          pipeline.AnnotateResult{SourceFile: "a.drawio", Marked: 0, Pages: 1, Changed: true},
			clean:      true,
			dryRun:     true,
			wantPrefix: "[DRY-RUN] ",
			wantSubstr: []string{"표식 0개", "요약 박스 1개", "제거 예정"},
		},
		{
			// Warnings는 «멈출 만큼은 아니지만 반드시 전해야 하는» 사실들이다.
			// 삼키면 사용자는 자기 파일에 무슨 일이 있었는지 영영 모른다 —
			// 결과 줄 밑에 그대로 찍는다.
			name: "경고가 있으면 결과 줄 밑에 찍는다",
			r: pipeline.AnnotateResult{
				SourceFile: "a.drawio", Marked: 1, Pages: 1, Changed: true,
				Warnings: []string{"셀 id \"t1\"이 페이지 pg1, pg2에 겹친다", "페이지 \"주문\"에 id가 없어 건너뛴다"},
			},
			clean:      true,
			wantPrefix: "[CLEANED] ",
			wantSubstr: []string{
				"표식 1개", "요약 박스 1개",
				"셀 id \"t1\"이 페이지 pg1, pg2에 겹친다",
				"페이지 \"주문\"에 id가 없어 건너뛴다",
			},
		},
		{
			// annotate 갈래에서도 같다 — 경고는 clean 전용이 아니다.
			name: "annotate 갈래의 경고도 찍는다",
			r: pipeline.AnnotateResult{
				SourceFile: "a.drawio", Marked: 0, Pages: 0, Findings: 1, Changed: false,
				Warnings: []string{"페이지 \"주문\"에 <diagram> id가 없어 건너뛴다"},
			},
			wantPrefix: "[UNCHANGED] ",
			wantSubstr: []string{"진단 1건", "페이지 \"주문\"에 <diagram> id가 없어 건너뛴다"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := annotateResultLine(c.r, c.clean, c.dryRun)
			if !strings.HasPrefix(got, c.wantPrefix) {
				t.Errorf("결과 = %q; want prefix %q", got, c.wantPrefix)
			}
			for _, s := range c.wantSubstr {
				if !strings.Contains(got, s) {
					t.Errorf("결과 = %q; want substring %q", got, s)
				}
			}
			for _, s := range c.wantNoParts {
				if strings.Contains(got, s) {
					t.Errorf("결과 = %q; 이 조각이 있으면 안 된다: %q", got, s)
				}
			}
		})
	}
}

// ---------------------------------------------------------------
// cmdFlags — 예전엔 여섯 서브커맨드에 복제돼 있었고 os.Exit을 안고 있어
// 단위 테스트가 불가능했다. 그 복제 때문에 runGenerate와 runWatch가
// 나란히 «모르는 플래그를 삼키고 exit 0»을 냈으니, 조용히 틀렸던 코드가
// 정확히 그물 밖에 있었던 셈이다. 이제 error를 돌려주므로 여기서 본다.
// ---------------------------------------------------------------

func TestCmdFlags_ParsesValuesAndSwitches(t *testing.T) {
	var cfg, out string
	var recursive, dry bool
	f := cmdFlags{
		strs:  map[string]strFlag{"--config": {&cfg, "path"}, "--out": {&out, "directory"}},
		bools: map[string]*bool{"--recursive": &recursive, "--dry-run": &dry},
	}

	rest, err := f.parse([]string{"--config", "c.yaml", "--recursive", "--out", "d"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("위치 인자가 남았다: %v", rest)
	}
	if cfg != "c.yaml" || out != "d" {
		t.Errorf("값 플래그가 안 담겼다: cfg=%q out=%q", cfg, out)
	}
	if !recursive {
		t.Error("--recursive가 안 켜졌다")
	}
	if dry {
		t.Error("주지도 않은 --dry-run이 켜졌다")
	}
}

// 값 플래그가 값 없이 끝나면 그 플래그와 «무엇을 기대했는지»를 말해야
// 한다. 같은 --out이라도 build는 파일을, convert는 폴더를 받으므로
// noun이 플래그마다 다르다.
func TestCmdFlags_MissingValueNamesTheFlagAndTheNoun(t *testing.T) {
	var out string
	f := cmdFlags{strs: map[string]strFlag{"--out": {&out, "directory"}}}

	_, err := f.parse([]string{"--out"})
	if err == nil {
		t.Fatal("에러가 없다 — 값을 빼먹은 플래그는 거부해야 한다")
	}
	if got, want := err.Error(), "--out requires a directory argument"; got != want {
		t.Errorf("문구=%q; %q여야 한다", got, want)
	}
}

// 모르는 플래그는 반드시 거부한다. 이 단언이 없어서 두 서브커맨드가
// 몇 달 동안 오타 난 플래그를 삼키고 exit 0을 냈다.
func TestCmdFlags_RejectsUnknownFlag(t *testing.T) {
	var recursive bool
	f := cmdFlags{bools: map[string]*bool{"--recursive": &recursive}}

	_, err := f.parse([]string{"--recursiv"})
	if err == nil {
		t.Fatal("오타 난 플래그가 통과했다")
	}
	if got, want := err.Error(), "unknown flag: --recursiv"; got != want {
		t.Errorf("문구=%q; %q여야 한다", got, want)
	}
}

// positional=false면 플래그가 아닌 인자도 거부한다 — generate/watch/
// convert/annotate는 대상 경로를 args[0]에서 이미 떼어 갔으므로 남는
// 위치 인자가 없어야 맞다.
func TestCmdFlags_RejectsStrayPositionalWhenNotAllowed(t *testing.T) {
	f := cmdFlags{}
	if _, err := f.parse([]string{"어쩌다.drawio"}); err == nil {
		t.Fatal("위치 인자가 통과했다")
	}
}

// positional=true면 모아서 돌려준다(build/reverse). 몇 개까지 받을지는
// 서브커맨드가 정한다 — 문구가 "input file"과 "target"으로 갈리기 때문이다.
func TestCmdFlags_CollectsPositionalsWhenAllowed(t *testing.T) {
	var out string
	f := cmdFlags{
		strs:       map[string]strFlag{"--out": {&out, "path"}},
		positional: true,
	}

	rest, err := f.parse([]string{"a.xlsx", "--out", "b.drawio", "c.xlsx"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := []string{"a.xlsx", "c.xlsx"}; !reflect.DeepEqual(rest, want) {
		t.Errorf("위치 인자=%v; %v여야 한다", rest, want)
	}
	if out != "b.drawio" {
		t.Errorf("out=%q; \"b.drawio\"여야 한다", out)
	}
}

// positional=true여도 «--»로 시작하면 모르는 플래그다. 이 갈래가 없으면
// 오타 난 플래그가 입력 파일 이름으로 조용히 접수된다.
func TestCmdFlags_PositionalModeStillRejectsUnknownFlags(t *testing.T) {
	f := cmdFlags{positional: true}
	if _, err := f.parse([]string{"a.xlsx", "--bogus"}); err == nil {
		t.Fatal("--bogus가 입력 파일로 접수됐다")
	}
}

// 왼쪽부터 읽다가 «첫 번째» 잘못에서 멈춘다. 모아서 나중에 보고하면
// 사용자가 고칠 곳과 화면에 뜬 이름이 어긋난다.
func TestCmdFlags_ReportsTheFirstProblem(t *testing.T) {
	f := cmdFlags{positional: true}
	_, err := f.parse([]string{"--first", "--second"})
	if err == nil {
		t.Fatal("에러가 없다")
	}
	if got, want := err.Error(), "unknown flag: --first"; got != want {
		t.Errorf("문구=%q; %q여야 한다 — 첫 번째 잘못을 말해야 한다", got, want)
	}
}

// 빈 인자 목록은 에러가 아니다 — 플래그를 하나도 안 준 실행이 정상이다.
func TestCmdFlags_EmptyArgsIsFine(t *testing.T) {
	rest, err := cmdFlags{}.parse(nil)
	if err != nil {
		t.Fatalf("빈 인자가 거부됐다: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("위치 인자=%v; 비어야 한다", rest)
	}
}

// version은 플래그도 위치 인자도 받지 않는다. runVersion이 쓰는 «빈»
// cmdFlags가 무엇이든 거부하는지 잡는다.
//
// 2026-09-22 이전에는 version만 예외였다 — `erdtool version --bogus`가
// 모르는 플래그를 조용히 무시하고 버전을 찍으며 종료 코드 0으로 끝났다.
// README는 그때도 「모르는 플래그는 어느 커맨드든 거부한다」고 적고 있었다.
func TestCmdFlags_EmptySetRejectsEverything(t *testing.T) {
	for _, arg := range []string{"--bogus-flag", "-v", "extra"} {
		if _, err := (cmdFlags{}).parse([]string{arg}); err == nil {
			t.Errorf("%q를 거부해야 한다 — version은 인자를 하나도 안 받는다", arg)
		}
	}
	// 인자가 없으면 통과해야 한다. 거부가 과하면 version 자체가 못 돈다.
	if _, err := (cmdFlags{}).parse(nil); err != nil {
		t.Errorf("인자가 없으면 통과해야 한다: %v", err)
	}
}
