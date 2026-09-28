package pipeline

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"erdtool/internal/config"
	"erdtool/internal/ddlcheck"
	"erdtool/internal/dialect"
	"erdtool/internal/ir"
	"erdtool/internal/report"
	"erdtool/internal/validate"
)

// reportDirSuffix는 파일 하나당 만들어지는 산출물 폴더의 접미사다. 재귀
// 스캔이 자기 산출물을 다시 읽지 않도록 같은 상수를 양쪽에서 쓴다.
const reportDirSuffix = "_report"

type Result struct {
	SourceFile string
	TableCount int
	OutputDir  string
}

// FileResult는 파일 하나의 처리 결과다. 폴더 일괄 생성(GenerateFolder)과
// watch가 같은 타입을 쓴다 — 둘 다 "파일 하나를 처리했고 성공했거나
// 실패했다"를 보고하는 일이라 형태가 같고, 화면에 찍는 코드도 하나로
// 유지된다.
type FileResult struct {
	SourceFile string
	Result     Result
	Err        error
}

// Generate는 파일 하나를 처리해 <파일명>_report/ 하위에 필수 산출물을 쓴다.
func Generate(path string, cfg config.Config) (Result, error) {
	doc, dups, err := ir.Assemble(path)
	if err != nil {
		return Result{}, err
	}

	// 타깃 DBMS는 여기서 한 번 푼다. 모르는 값이면 산출물을 하나도 안 만들고
	// 멈춘다 — 오타(`--dialect postgre`)를 조용히 ANSI로 떨어뜨리면 사용자는
	// 자기가 지정한 대로 뽑혔다고 믿는다.
	target, err := dialect.Parse(cfg.Dialect)
	if err != nil {
		return Result{}, err
	}
	opt := report.Options{PageAsDomain: cfg.PageAsDomain, Dialect: target}

	required := validate.Required(doc, dups)
	optional := validate.Optional(doc, cfg)
	findings := append(required, optional...)
	// 시험 실행(dry run)은 앞의 둘이 끝난 뒤에 돈다 — 구조가 이미 진단이 난 테이블은
	// 건너뛰어서 같은 실수가 두 줄로 보고되지 않게 하기 위함이다.
	findings = append(findings, ddlcheck.DryRun(doc, findings, opt)...)

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	outDir := filepath.Join(filepath.Dir(path), base+reportDirSuffix)
	if cfg.OutputDir != "" {
		// cfg.OutputDir는 "{basename}" 플레이스홀더를 base로 치환한 뒤
		// 파일이 위치한 디렉터리를 기준으로 해석한다.
		resolved := strings.ReplaceAll(cfg.OutputDir, "{basename}", base)
		if filepath.IsAbs(resolved) {
			outDir = resolved
		} else {
			outDir = filepath.Join(filepath.Dir(path), resolved)
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, err
	}

	// 테이블정의서는 원장에서 필수 산출물이며 MD/HTML/PDF/xlsx 네 포맷 전부를
	// 항상 생성한다(관계정의서/SQL DDL만 옵션).
	if err := os.WriteFile(filepath.Join(outDir, "table_doc.md"),
		[]byte(report.TableDocMarkdown(doc, opt)), 0o644); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "table_doc.html"),
		[]byte(report.TableDocHTML(doc, opt)), 0o644); err != nil {
		return Result{}, err
	}
	xlsxFile, err := report.TableDocXLSX(doc, opt)
	if err != nil {
		return Result{}, err
	}
	// excelize.File은 임시 리소스를 들고 있어 명시적으로 Close해야 한다 —
	// 안 그러면 generate 1회, watch로 재생성될 때마다 계속 누적된다.
	saveErr := xlsxFile.SaveAs(filepath.Join(outDir, "table_doc.xlsx"))
	closeErr := xlsxFile.Close()
	if saveErr != nil {
		return Result{}, saveErr
	}
	if closeErr != nil {
		return Result{}, closeErr
	}
	if err := report.TableDocPDF(doc, filepath.Join(outDir, "table_doc.pdf"), opt); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "validation_report.md"),
		[]byte(report.ValidationMarkdown(doc, findings, opt)), 0o644); err != nil {
		return Result{}, err
	}
	if err := ir.Write(doc, filepath.Join(outDir, "ir.json")); err != nil {
		return Result{}, err
	}

	if cfg.Outputs.RelationDoc {
		if err := os.WriteFile(filepath.Join(outDir, "relation_doc.md"),
			[]byte(report.RelationDocMarkdown(doc)), 0o644); err != nil {
			return Result{}, err
		}
	}
	if cfg.Outputs.SQLDDL {
		if err := os.WriteFile(filepath.Join(outDir, "schema.sql"),
			[]byte(report.SQLDDL(doc, opt)), 0o644); err != nil {
			return Result{}, err
		}
	}

	tableCount := 0
	for _, d := range doc.Diagrams {
		tableCount += len(d.Tables)
	}

	return Result{SourceFile: path, TableCount: tableCount, OutputDir: outDir}, nil
}

// GenerateFolder는 디렉터리 안의 *.drawio 전체를 처리한다. 파일 하나의
// 실패가 나머지를 막지 않는다(원장: 폴더 배치 처리의 파일 단위 에러 격리).
//
// 확장자 비교는 대소문자를 구분하지 않는다. filepath.Glob("*.drawio")는
// 패턴 매칭이 대소문자를 구분해서, Linux/macOS에서 ".DRAWIO"로 저장된 파일이
// 에러도 경고도 없이 통째로 건너뛰어졌다 — 사용자 입장에서는 산출물만 안
// 나오고 이유를 알 길이 없다.
//
// 하위 폴더는 cfg.Recursive일 때만 들어간다(기본 꺼짐).
func GenerateFolder(dir string, cfg config.Config) ([]FileResult, error) {
	paths, err := findDrawioFiles(dir, cfg.Recursive)
	if err != nil {
		return nil, err
	}
	var results []FileResult
	for _, path := range paths {
		r, genErr := Generate(path, cfg)
		results = append(results, FileResult{SourceFile: path, Result: r, Err: genErr})
	}
	return results, nil
}

// findDrawioFiles는 처리 대상 .drawio 목록을 돌려준다. watch도 «폴더째»
// 들어온 파일을 찾을 때 같은 함수를 쓴다 — 두 경로가 서로 다른 규칙으로
// 파일을 고르면 결과가 조용히 갈린다.
func findDrawioFiles(dir string, recursive bool) ([]string, error) {
	isDrawio := func(name string) bool {
		return strings.EqualFold(filepath.Ext(name), ".drawio")
	}
	var paths []string

	if !recursive {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !isDrawio(e.Name()) {
				continue
			}
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
		return paths, nil
	}

	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			// 읽을 수 없는 폴더 하나가 나머지를 막지 않는다 — 파일 단위
			// 에러 격리와 같은 이유다.
			return nil
		}
		if e.IsDir() {
			// 자기가 만든 산출물 폴더에는 들어가지 않는다. 지금은 그 안에
			// .drawio가 없지만, output_dir를 트리 안쪽으로 돌려놓으면 방금
			// 만든 것을 다시 읽는 일이 생긴다.
			if path != dir && strings.HasSuffix(e.Name(), reportDirSuffix) {
				return fs.SkipDir
			}
			return nil
		}
		if isDrawio(e.Name()) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}
