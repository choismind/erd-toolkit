package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"erdtool/internal/config"
)

func TestGenerate_SingleFile_ProducesAllRequiredOutputs(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "relationship_logical.drawio")
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", src)

	result, err := Generate(src, config.Config{})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	outDir := filepath.Join(tmp, "relationship_logical_report")
	// 원장: 테이블정의서는 MD/HTML/PDF/xlsx 네 포맷 전부가 필수 산출물이다.
	for _, want := range []string{
		"table_doc.md", "table_doc.html", "table_doc.pdf", "table_doc.xlsx",
		"validation_report.md", "ir.json",
	} {
		p := filepath.Join(outDir, want)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected output %s to exist: %v", want, err)
		}
	}
	if result.TableCount == 0 {
		t.Fatalf("expected non-zero table count in result summary")
	}
}

func TestGenerate_HonorsConfigOutputDir(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "relationship_logical.drawio")
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", src)

	cfg := config.Config{OutputDir: "custom_{basename}_out"}
	result, err := Generate(src, cfg)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	wantDir := filepath.Join(tmp, "custom_relationship_logical_out")
	if result.OutputDir != wantDir {
		t.Fatalf("expected output dir %s, got %s", wantDir, result.OutputDir)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "table_doc.md")); err != nil {
		t.Fatalf("expected table_doc.md in custom output dir: %v", err)
	}
}

func TestGenerate_Folder_IsolatesPerFileFailures(t *testing.T) {
	tmp := t.TempDir()
	good := filepath.Join(tmp, "good.drawio")
	bad := filepath.Join(tmp, "bad.drawio")
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", good)
	os.WriteFile(bad, []byte("not even xml"), 0o644)

	results, err := GenerateFolder(tmp, config.Config{})
	if err != nil {
		t.Fatalf("GenerateFolder should not fail entirely on one bad file: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 per-file results, got %d", len(results))
	}
	var sawError, sawSuccess bool
	for _, r := range results {
		if r.Err != nil {
			sawError = true
		} else {
			sawSuccess = true
		}
	}
	if !sawError || !sawSuccess {
		t.Fatalf("expected one failure and one success, got: %+v", results)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func TestGenerateFolder_MatchesExtensionCaseInsensitively(t *testing.T) {
	// filepath.Glob("*.drawio")는 대소문자를 구분한다. Linux/macOS에서
	// ".DRAWIO"/".Drawio"로 저장된 파일이 조용히 통째로 건너뛰어진다 —
	// 에러도 경고도 없이 산출물만 안 나온다.
	tmp := t.TempDir()
	for _, name := range []string{"lower.drawio", "upper.DRAWIO", "mixed.DrawIO"} {
		copyFile(t, "../drawio/testdata/relationship_logical.drawio", filepath.Join(tmp, name))
	}
	// 확장자가 다른 파일은 여전히 제외되어야 한다.
	if err := os.WriteFile(filepath.Join(tmp, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	results, err := GenerateFolder(tmp, config.Config{})
	if err != nil {
		t.Fatalf("GenerateFolder failed: %v", err)
	}
	if len(results) != 3 {
		var names []string
		for _, r := range results {
			names = append(names, filepath.Base(r.SourceFile))
		}
		t.Fatalf("expected 3 .drawio files regardless of extension case, got %d: %v", len(results), names)
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("%s failed: %v", filepath.Base(r.SourceFile), r.Err)
		}
	}
}

func TestGenerateFolder_IgnoresSubdirectories(t *testing.T) {
	// 폴더 재귀 스캔은 아직 지원하지 않는다(I9로 별도 논의). 하위 폴더의
	// 파일을 조용히 처리해버리지 않는다는 것만 고정해 둔다.
	tmp := t.TempDir()
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", filepath.Join(tmp, "top.drawio"))
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	copyFile(t, "../drawio/testdata/relationship_logical.drawio", filepath.Join(sub, "nested.drawio"))

	results, err := GenerateFolder(tmp, config.Config{})
	if err != nil {
		t.Fatalf("GenerateFolder failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected only the top-level file, got %d results", len(results))
	}
}

func TestGenerateFolder_RecursiveOnlyWhenAsked(t *testing.T) {
	// 기본은 비재귀다(사용자 결정). 큰 트리를 잘못 지정했을 때 조용히
	// 오래 도는 쪽보다, 필요할 때만 켜는 쪽이 사고가 없다.
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub", "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", filepath.Join(tmp, "top.drawio"))
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", filepath.Join(sub, "nested.drawio"))

	cfg := config.Config{}
	flat, err := GenerateFolder(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(flat) != 1 {
		t.Fatalf("기본은 현재 폴더만 처리해야 한다: %d개 처리됨", len(flat))
	}

	cfg.Recursive = true
	deep, err := GenerateFolder(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(deep) != 2 {
		t.Fatalf("recursive면 하위 폴더까지 처리해야 한다: %d개 처리됨", len(deep))
	}
}

func TestGenerateFolder_RecursiveSkipsItsOwnOutput(t *testing.T) {
	// 재귀는 자기가 만든 산출물 폴더도 다시 읽는다. 산출물에 .drawio는
	// 없지만, output_dir를 다른 폴더로 돌려놨다면 그 안의 .drawio를
	// 재처리하는 일이 생길 수 있다 — 적어도 산출물 폴더 자체는 건너뛴다.
	tmp := t.TempDir()
	copyFile(t, "../drawio/testdata/entity_table_basic.drawio", filepath.Join(tmp, "a.drawio"))

	cfg := config.Config{Recursive: true}
	if _, err := GenerateFolder(tmp, cfg); err != nil {
		t.Fatal(err)
	}
	// 산출물이 생긴 상태에서 한 번 더 — 처리 대상 수가 늘어나면 안 된다.
	again, err := GenerateFolder(tmp, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 {
		t.Fatalf("두 번째 실행에서 대상이 늘었다: %d개", len(again))
	}
}
