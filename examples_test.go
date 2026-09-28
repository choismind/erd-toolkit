// examples_test.go — examples/에 넣어 둔 산출물이 지금 코드와 맞는지 본다.
//
// 이 파일이 있는 이유: `examples/`는 저장소를 처음 연 사람이 **돌려 보기
// 전에 먼저 읽는** 자리다. 거기 커밋해 둔 정의서와 DDL이 코드보다 낡으면,
// 방문자가 보는 첫 산출물이 거짓이 된다. 그리고 그 거짓은 아무 데서도
// 안 터진다 — 예제를 다시 뽑아 보는 사람이 없기 때문이다.
//
// 그래서 여기서 다시 뽑아 맞대 본다. 어긋나면 이렇게 되살린다:
//
//	go build -o erdtool.exe ./cmd/erdtool
//	./erdtool.exe build examples/bookstore.csv
//	./erdtool.exe generate examples/bookstore.drawio --relations --sql
//	# examples/README.md의 「산출물 여덟」 표에서 커밋하지 않는 파일은 지운다
//
// 되살릴 때는 «릴리스에서 받은 바이너리»가 아니라 **위처럼 이 저장소에서
// 빌드한 것**을 쓴다. 정의서 머리의 「생성 도구」에 버전이 박히는데, 릴리스
// 바이너리는 태그에서 뽑은 값(`erdtool 0.1.0`)이고 여기서 빌드한 것은
// 기본값(`erdtool 0.1.0-dev`)이다. 이 테스트는 pipeline을 바로 불러 재므로
// 언제나 기본값 쪽과 맞는다 — 릴리스로 뽑아 커밋하면 그 한 줄 때문에
// 이 테스트가 깨진다(2026-09-22에 WSL2로 재 보다 드러났다).
package erdtool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"erdtool/internal/config"
	"erdtool/internal/pipeline"
)

// exampleReportFiles는 `examples/bookstore_report/`에 커밋해 둔 것이다.
// `.html`·`.xlsx`·`.pdf`도 함께 나오지만 커밋하지 않는다 — 셋 다 이진이거나
// 긴 한 줄이라 diff가 읽히지 않고, 같은 내용을 `.md`가 이미 싣는다.
var exampleReportFiles = []string{
	"table_doc.md",
	"relation_doc.md",
	"validation_report.md",
	"schema.sql",
	"ir.json",
}

// volatileLineRE는 실행할 때마다 달라지는 줄이다. 「기준 시각」은 원본
// `.drawio`가 마지막으로 바뀐 시각이고, git은 파일 시각을 보존하지 않으므로
// 받아 간 사람마다 다른 값이 나온다. 비교에서 그 줄만 뺀다.
var volatileLineRE = regexp.MustCompile(`(?m)^.*(기준 시각|source_modified).*$`)

func readExample(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s 읽기: %v", path, err)
	}
	return volatileLineRE.ReplaceAllString(strings.ReplaceAll(string(raw), "\r\n", "\n"), "")
}

// TestExampleDiagramMatchesDesignSheet는 커밋된 `bookstore.drawio`가
// 커밋된 `bookstore.csv`에서 지금도 그대로 나오는지 본다. `build`의 출력은
// 시각을 안 싣고 셀 id를 순서대로 매기므로 바이트까지 같아야 한다.
func TestExampleDiagramMatchesDesignSheet(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bookstore.drawio")
	if _, err := pipeline.Build(filepath.Join("examples", "bookstore.csv"), out); err != nil {
		t.Fatalf("build: %v", err)
	}

	got := readExample(t, out)
	want := readExample(t, filepath.Join("examples", "bookstore.drawio"))
	if got != want {
		t.Errorf("examples/bookstore.drawio가 examples/bookstore.csv에서 나오는 것과 다르다.\n" +
			"설계서나 build를 고쳤으면 예제도 다시 뽑아라 — 되살리는 명령은 이 파일 첫머리에 있다")
	}
}

// TestExampleReportMatchesDiagram은 커밋된 산출물이 커밋된 `.drawio`에서
// 지금도 그대로 나오는지 본다. 이쪽이 깨지는 것은 대개 **예제를 고쳐서가
// 아니라 리포터를 고쳐서**다.
func TestExampleReportMatchesDiagram(t *testing.T) {
	var cfg config.Config
	cfg.Outputs.RelationDoc = true
	cfg.Outputs.SQLDDL = true
	cfg.OutputDir = t.TempDir() // 절대 경로라 입력 파일 옆이 아니라 여기로 쓴다

	// ir.json의 source_file에 «준 경로»가 그대로 박히므로, 커밋된 것과 같은
	// 표기(슬래시)로 넘긴다.
	if _, err := pipeline.Generate("examples/bookstore.drawio", cfg); err != nil {
		t.Fatalf("generate: %v", err)
	}

	for _, name := range exampleReportFiles {
		got := readExample(t, filepath.Join(cfg.OutputDir, name))
		want := readExample(t, filepath.Join("examples", "bookstore_report", name))
		if got != want {
			t.Errorf("examples/bookstore_report/%s가 지금 코드가 내는 것과 다르다.\n"+
				"리포터를 고쳤으면 예제도 다시 뽑아라 — 되살리는 명령은 이 파일 첫머리에 있다", name)
		}
	}
}

// TestExampleReportHasNoStrayFiles는 커밋 목록에 없는 파일이 예제 폴더에
// 섞여 들어가지 않았는지 본다. `.pdf`·`.xlsx`를 실수로 커밋하면 다음
// 재생성마다 이진 diff가 쌓인다.
func TestExampleReportHasNoStrayFiles(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("examples", "bookstore_report"))
	if err != nil {
		t.Fatalf("예제 산출물 폴더 읽기: %v", err)
	}
	want := map[string]bool{}
	for _, name := range exampleReportFiles {
		want[name] = true
	}
	for _, e := range entries {
		if !want[e.Name()] {
			t.Errorf("examples/bookstore_report/%s는 커밋 목록에 없다", e.Name())
		}
	}
	if len(entries) != len(exampleReportFiles) {
		t.Errorf("예제 산출물이 %d개인데 커밋 목록은 %d개다", len(entries), len(exampleReportFiles))
	}
}
