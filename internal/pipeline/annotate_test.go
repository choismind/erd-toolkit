// internal/pipeline/annotate_test.go
package pipeline

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"erdtool/internal/config"
	"erdtool/internal/ir"
	"erdtool/internal/validate"
)

// fixtureWithMissingPK/fixtureClean은 internal/annotate 패키지 테스트의
// 것과 같은 내용이다(패키지가 달라 그쪽 상수를 그대로 쓸 수 없다).

// fixtureWithMissingPK는 PK 없는 테이블 하나짜리 페이지다.
const fixtureWithMissingPK = `<mxfile><diagram name="주문" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="주문" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="주문일자 DATE" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// fixtureClean은 진단이 하나도 안 나오는 페이지다.
const fixtureClean = `<mxfile><diagram name="고객" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="CUSTOMER" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="CUST_ID INTEGER" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// fixtureClean이 실제로 진단 0건인지부터 확인한다. 브리프가 "그렇다는
// 주장을 믿지 말고 먼저 검증하라"고 못 박았다 — 이 상수는 손으로 옮겨
// 적은 것이라 원본과 조용히 달라졌을 수 있다.
func TestFixtureCleanHasNoFindings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.drawio")
	if err := os.WriteFile(p, []byte(fixtureClean), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := config.Config{}
	doc, dups, err := ir.Assemble(p)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	findings := append(validate.Required(doc, dups), validate.Optional(doc, cfg)...)
	if len(findings) != 0 {
		t.Fatalf("fixtureClean에 진단이 %d건 있다; 0건이어야 한다: %v", len(findings), findings)
	}
}

func TestAnnotateWritesInPlace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "주문.drawio")
	if err := os.WriteFile(p, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	before, _ := os.ReadFile(p)

	r, err := Annotate(p, config.Config{}, false, false)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if !r.Changed || r.Marked == 0 {
		t.Fatalf("아무것도 안 바뀌었다: %+v", r)
	}
	after, _ := os.ReadFile(p)
	if bytes.Equal(before, after) {
		t.Error("제자리 쓰기가 안 됐다 — 파일이 그대로다")
	}
	// 새 파일을 만들지 않는다.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("파일 %d개; 1개여야 한다 (제자리 수정이다)", len(entries))
	}
}

func TestAnnotateDryRunWritesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "주문.drawio")
	if err := os.WriteFile(p, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	before, _ := os.ReadFile(p)

	r, err := Annotate(p, config.Config{}, false, true)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Marked == 0 {
		t.Error("--dry-run도 무엇을 마크할지는 세야 한다")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Error("--dry-run인데 파일이 바뀌었다")
	}
}

// 진단이 없으면 mtime도 안 건드린다.
func TestAnnotateDoesNotTouchCleanFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "고객.drawio")
	if err := os.WriteFile(p, []byte(fixtureClean), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	st1, _ := os.Stat(p)

	r, err := Annotate(p, config.Config{}, false, false)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Changed {
		t.Error("Changed=true; 진단이 없으면 false여야 한다")
	}
	st2, _ := os.Stat(p)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Error("진단이 없는데 mtime이 바뀌었다 — 쓰지 말아야 한다")
	}
}

// 위 테스트의 거울상: 정말로 진단이 있는 파일은 mtime이 바뀌어야 한다.
// 이게 없으면 "mtime 안 바뀜" 테스트가 코드가 아예 안 써서 통과하는
// 것인지, 판단해서 안 쓰는 것인지 구별이 안 된다.
func TestAnnotateTouchesMtimeWhenChanged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "주문.drawio")
	if err := os.WriteFile(p, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	st1, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// 파일시스템의 mtime 해상도가 낮을 수 있어(FAT32 등) 확실히
	// 앞서도록 조금 재운다.
	newTime := st1.ModTime().Add(-2 * time.Second)
	if err := os.Chtimes(p, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	st1, _ = os.Stat(p)

	r, err := Annotate(p, config.Config{}, false, false)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if !r.Changed {
		t.Fatalf("Changed=false; 진단이 있는데 안 바뀌었다고 한다: %+v", r)
	}
	st2, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st1.ModTime().Equal(st2.ModTime()) {
		t.Error("진단이 있어 파일을 썼는데 mtime이 그대로다")
	}
}

// --dry-run과 --clean을 같이 주면: 뭘 지울지는 세되(annotate.Clean이
// 표식이 있는 입력에서 Changed=true를 돌려주는 경우), 파일에는 손대지
// 않는다.
func TestAnnotateDryRunWithCleanWritesNothing(t *testing.T) {
	dir := t.TempDir()
	marked := filepath.Join(dir, "주문.drawio")
	if err := os.WriteFile(marked, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// 먼저 진짜로 표식을 남긴다(clean이 지울 대상을 만든다).
	if _, err := Annotate(marked, config.Config{}, false, false); err != nil {
		t.Fatalf("사전 Annotate: %v", err)
	}
	before, err := os.ReadFile(marked)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	st1, _ := os.Stat(marked)

	r, err := Annotate(marked, config.Config{}, true, true)
	if err != nil {
		t.Fatalf("Annotate(clean, dryRun): %v", err)
	}
	if !r.Changed {
		t.Fatalf("표식이 있는 파일인데 --clean --dry-run이 Changed=false라고 한다: %+v", r)
	}
	after, _ := os.ReadFile(marked)
	if !bytes.Equal(before, after) {
		t.Error("--dry-run인데 --clean이 파일을 건드렸다")
	}
	st2, _ := os.Stat(marked)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Error("--dry-run인데 mtime이 바뀌었다")
	}
}

// 폴더 일괄 처리에서 파일 하나가 실패해도 나머지는 계속 처리된다.
// generate/convert와 같은 파일 단위 에러 격리다 — 통째 중단은 이
// 서브커맨드에서도 틀린 모양이다.
func TestAnnotateFolder_IsolatesPerFileFailures(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.drawio")
	bad := filepath.Join(dir, "bad.drawio")
	if err := os.WriteFile(good, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write good: %v", err)
	}
	if err := os.WriteFile(bad, []byte("not even xml"), 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}

	results, err := AnnotateFolder(dir, config.Config{}, false)
	if err != nil {
		t.Fatalf("AnnotateFolder는 파일 하나의 실패로 통째 실패하면 안 된다: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("파일별 결과 %d개; 2개여야 한다", len(results))
	}
	var sawError, sawSuccess bool
	for _, r := range results {
		if r.Err != nil {
			sawError = true
		} else if r.Changed {
			sawSuccess = true
		}
	}
	if !sawError {
		t.Errorf("bad.drawio가 에러로 안 잡혔다: %+v", results)
	}
	if !sawSuccess {
		t.Errorf("good.drawio가 처리되지 않았다: %+v", results)
	}

	afterGood, _ := os.ReadFile(good)
	beforeGood := []byte(fixtureWithMissingPK)
	if bytes.Equal(beforeGood, afterGood) {
		t.Error("good.drawio가 실제로는 안 바뀌었다")
	}
	afterBad, _ := os.ReadFile(bad)
	if !bytes.Equal(afterBad, []byte("not even xml")) {
		t.Error("bad.drawio는 실패했으니 원본 그대로여야 한다")
	}
}

// writeFileAtomic이 기존 파일을 실제로 새 내용으로 바꿔치기하고,
// 그 과정에서 만든 임시 파일을 디렉터리에 남기지 않는지 확인한다.
// 대상이 이미 존재하는 일반 파일이므로 이건 곧 "Windows에서
// os.Rename이 기존 파일을 덮어쓰는가"의 실증이기도 하다 — 통제자의
// 판단이 이 플랫폼에서 실제로 성립하는지는 말이 아니라 이 테스트가
// 보증한다.
func TestWriteFileAtomicReplacesContentAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.drawio")
	if err := os.WriteFile(p, []byte("old content"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := writeFileAtomic(p, []byte("new content"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("내용이 %q; new content여야 한다", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("디렉터리에 항목 %d개; 대상 파일 하나여야 한다(임시 파일이 남았다): %+v", len(entries), entries)
	}
}

// rename이 실패하는 실제 상황을 만든다: 대상 경로가 이미 디렉터리인
// 경우, 파일로 그 위에 rename하는 것은 POSIX·Windows 양쪽에서 거부된다
// (Windows는 MoveFileEx가 ERROR_ACCESS_DENIED류로, POSIX는 EISDIR로).
// 이때 임시 파일은 rename 대상 디렉터리가 아니라 그 부모(=대상 경로의
// filepath.Dir)에 만들어지므로, 실패는 "임시 파일 쓰기·동기화·닫기는
// 이미 끝났고 rename 한 걸음만 남은" 지점에서 일어난다 — 즉 rename 전
// 구간(pre-rename window)에서의 실패를 그대로 재현한다. 대상 디렉터리
// 안의 파일(sentinel)이 그대로인지로 "원본 무손상"을, 부모 디렉터리
// 목록으로 "임시 파일 잔재 없음"을 확인한다.
func TestWriteFileAtomicCleansUpOnRenameFailure(t *testing.T) {
	parent := t.TempDir()
	targetDir := filepath.Join(parent, "target.drawio")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sentinel := filepath.Join(targetDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("원본 그대로"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	err := writeFileAtomic(targetDir, []byte("new content"), 0o644)
	if err == nil {
		t.Fatal("디렉터리 위로 rename이 성공했다 — 실패해야 하는 상황이다")
	}

	after, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("sentinel이 사라졌다: %v", err)
	}
	if string(after) != "원본 그대로" {
		t.Errorf("sentinel 내용이 바뀌었다: %q", after)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("readdir parent: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "target.drawio" {
		t.Errorf("부모 디렉터리에 임시 파일이 남았다: %+v", entries)
	}
}

// 하위 폴더는 cfg.Recursive일 때만 들어간다. generate/convert와 같은
// 규칙을 findDrawioFiles로 공유하므로, 여기서는 그 공유가 실제로
// annotate 경로에도 적용됐는지만 확인한다.
func TestAnnotateFolder_RecursiveOnlyWhenAsked(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	top := filepath.Join(dir, "top.drawio")
	nested := filepath.Join(sub, "nested.drawio")
	if err := os.WriteFile(top, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write top: %v", err)
	}
	if err := os.WriteFile(nested, []byte(fixtureWithMissingPK), 0o644); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	results, err := AnnotateFolder(dir, config.Config{Recursive: false}, false)
	if err != nil {
		t.Fatalf("AnnotateFolder: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("비재귀인데 결과 %d개; 1개여야 한다: %+v", len(results), results)
	}

	results, err = AnnotateFolder(dir, config.Config{Recursive: true}, false)
	if err != nil {
		t.Fatalf("AnnotateFolder(recursive): %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("재귀인데 결과 %d개; 2개여야 한다: %+v", len(results), results)
	}
}

// fixtureDDLOnly는 구조 검사는 다 통과하고 DDL 문법 검사만 걸리는 페이지다.
// enum(...)은 MySQL에는 있지만 시험 실행에 쓰는 SQLite에는 없다.
const fixtureDDLOnly = `<mxfile><diagram name="영화" id="pg1"><mxGraphModel><root>
  <mxCell id="0"/><mxCell id="1" parent="0"/>
  <mxCell id="t1" value="FILM" style="shape=table;childLayout=tableLayout;" vertex="1" parent="1">
    <mxGeometry x="80" y="40" width="240" height="60" as="geometry"/>
  </mxCell>
  <mxCell id="r1" style="shape=tableRow;" vertex="1" parent="t1">
    <mxGeometry y="30" width="240" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1k" value="PK" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry width="30" height="30" as="geometry"/>
  </mxCell>
  <mxCell id="r1d" value="RATING enum('G','PG')" style="shape=partialRectangle;" vertex="1" parent="r1">
    <mxGeometry x="30" width="210" height="30" as="geometry"/>
  </mxCell>
</root></mxGraphModel></diagram></mxfile>`

// annotate는 generate가 내는 진단을 «전부» 그림에 되돌려야 한다. 2026-09-22
// 이전에는 ddlcheck를 안 불러서 ddl_syntax만 뜬 파일에 annotate를 돌리면
// 아무 일도 안 일어났다 — 리포트는 1건이라는데 annotate는 「진단 0건」이라
// 말했고, 사용자가 그 차이를 알 방법이 없었다.
func TestAnnotateMarksDDLSyntaxFindings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "영화.drawio")
	if err := os.WriteFile(p, []byte(fixtureDDLOnly), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r, err := Annotate(p, config.Config{}, false, false)
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if r.Findings == 0 {
		t.Fatal("진단 0건 — ddlcheck를 안 돌린 것이다")
	}
	if r.Marked == 0 {
		t.Errorf("마크 0개; ddl_syntax에는 붙일 셀 id가 있다: %+v", r)
	}
}

// 같은 파일을 두 서브커맨드에 먹이면 진단 건수가 같아야 한다. 이 계약이
// 깨지면 리포트를 보고 annotate를 돌린 사용자가 «왜 숫자가 다르지»에서
// 멈춘다.
func TestAnnotateAndGenerateSeeTheSameFindings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "영화.drawio")
	if err := os.WriteFile(p, []byte(fixtureDDLOnly), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	gen, err := Generate(p, config.Config{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// 사용자가 실제로 보는 숫자는 리포트의 「진단 N건」이다.
	rep, err := os.ReadFile(filepath.Join(gen.OutputDir, "validation_report.md"))
	if err != nil {
		t.Fatalf("리포트 읽기: %v", err)
	}
	m := regexp.MustCompile(`진단 (\d+)건`).FindSubmatch(rep)
	if m == nil {
		t.Fatalf("리포트에 진단 건수가 없다:\n%s", rep)
	}
	want, _ := strconv.Atoi(string(m[1]))

	ann, err := Annotate(p, config.Config{}, false, true) // dry-run: 파일을 안 건드린다
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if ann.Findings != want {
		t.Errorf("리포트=%d건, annotate=%d건; 같아야 한다", want, ann.Findings)
	}
}
