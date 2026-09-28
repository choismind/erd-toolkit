package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"erdtool/internal/genbuild"
)

type BuildFileResult struct {
	InputPath   string
	OutputPath  string
	Overwritten bool
	Pages       int
	Tables      int
}

// OutputPath는 입력 경로의 확장자를 .drawio로 바꾼다.
// "설계서.xlsx" -> "설계서.drawio".
func OutputPath(inputPath string) string {
	ext := filepath.Ext(inputPath)
	return strings.TrimSuffix(inputPath, ext) + ".drawio"
}

// Build는 xlsx/csv 데이터 설계서 파일 하나를 읽어 .drawio를 쓴다.
// outPath가 빈 문자열이면 OutputPath(inputPath)를 쓴다.
func Build(inputPath, outPath string) (BuildFileResult, error) {
	src, err := os.ReadFile(inputPath)
	if err != nil {
		return BuildFileResult{}, fmt.Errorf("입력 읽기 실패: %w", err)
	}

	var pages []genbuild.PageDef
	switch strings.ToLower(filepath.Ext(inputPath)) {
	case ".xlsx":
		pages, err = genbuild.ParseXLSX(src)
	case ".csv":
		base := filepath.Base(inputPath)
		pageName := strings.TrimSuffix(base, filepath.Ext(base))
		pages, err = genbuild.ParseCSV(pageName, src)
	default:
		return BuildFileResult{}, fmt.Errorf("지원하지 않는 확장자: %s (xlsx/csv만 지원)", filepath.Ext(inputPath))
	}
	if err != nil {
		return BuildFileResult{}, err
	}

	out, err := genbuild.Build(pages)
	if err != nil {
		return BuildFileResult{}, err
	}

	if outPath == "" {
		outPath = OutputPath(inputPath)
	}
	// 데이터 설계서도 convert.go의 논리 ERD와 같은 처지다 — 사람이 손으로
	// 채운 원본이며, --out을 입력과 같은 경로로 줘서 build가 그 위에 drawio
	// XML을 덮어쓰면 잃은 뒤 되돌릴 길이 없다(리뷰에서 발견).
	if err := guardNotOverwriting(inputPath, outPath); err != nil {
		return BuildFileResult{}, err
	}
	_, statErr := os.Stat(outPath)
	overwritten := statErr == nil

	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return BuildFileResult{}, fmt.Errorf("출력 쓰기 실패: %w", err)
	}

	tableCount := 0
	for _, p := range pages {
		tableCount += len(p.Tables)
	}

	return BuildFileResult{
		InputPath: inputPath, OutputPath: outPath, Overwritten: overwritten,
		Pages: len(pages), Tables: tableCount,
	}, nil
}
