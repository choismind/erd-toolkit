// internal/pipeline/convert.go
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"erdtool/internal/config"
	"erdtool/internal/convert"
	"erdtool/internal/glossary"
)

// physicalStemSuffix는 convert가 만든 출력 파일의 확장자 앞 접미사다
// (convert.OutputPath와 같은 규칙: X.drawio -> X.physical.drawio).
// ConvertFolder가 자기 산출물을 다시 입력으로 삼지 않도록 폴더를 읽을 때
// 이 접미사를 걸러낸다.
const physicalStemSuffix = ".physical"

// ConvertFileResult는 폴더 일괄 변환에서 파일 하나의 처리 결과다.
// generate 쪽 FileResult(SourceFile/Result/Err)와 형태만 닮았을 뿐
// 실제로 담는 값이 다르다 — convert에는 TableCount/OutputDir 개념이
// 없고, 출력 파일 경로(OutputFile)와 변환 통계(Stats)를 담는다.
type ConvertFileResult struct {
	SourceFile string
	OutputFile string
	Stats      convert.Stats
	// Overwritten은 쓰기 «전»에 이미 그 경로에 파일이 있었는지다.
	//
	// 스펙 결정 5는 convert를 별도 서브커맨드로 둔 이유를 "중간 산출물(물리
	// ERD)을 사람이 검수·수정한 다음 리포트를 만들 수 있어야" 한다고 못
	// 박았다. 그런데 convert를 다시 돌리면 그 손댄 파일을 아무 신호 없이
	// 덮어쓴다. logicalName은 «이름»만 지켜준다 — 레이아웃, 손으로 더한
	// 컬럼, 메모는 아무것도 지켜주지 못한다.
	//
	// 그렇다고 --force 같은 관문을 두지는 않는다. 멱등성이 설계 목표라
	// 재실행은 정상 사용이고, 두 번째 실행을 실패시키면 그 목표와 싸운다.
	// 대신 «침묵을 신호로» 바꾼다 — 호출자가 [CONVERTED]와 [OVERWRITTEN]을
	// 가려 찍어서, 사용자가 매 실행마다 자기 파일이 대체됐음을 듣는다.
	Overwritten bool
	Err         error
}

// Convert는 논리 .drawio 파일 하나를 물리 .drawio로 바꿔 쓴다.
// 반환값의 둘째는 실제로 쓴 출력 경로, 셋째는 그 경로에 이미 파일이
// 있었는지(ConvertFileResult.Overwritten과 같은 뜻)다.
func Convert(path string, dictPath, outDir string) (convert.Stats, string, bool, error) {
	d, err := glossary.Load(dictPath)
	if err != nil {
		return convert.Stats{}, "", false, err
	}
	return convertWithDict(path, d, outDir)
}

// ConvertStats는 파일 하나를 «쓰지 않고» 변환해 통계만 돌려준다.
// convert --dry-run이 쓴다.
//
// 이 진입점이 있는 이유: 예전에는 cmd/erdtool이 dry-run 경로에서
// os.ReadFile + glossary.Load + convert.File을 **직접 다시 조립**했다.
// 그러면서 에러 문구를 아래 loadAndConvert의 것과 손으로 맞춰 놓은
// 주석까지 달려 있었다 — 「한쪽만 고치면 dry-run과 실제 실행이 같은
// 실패를 다른 정보량으로 보고한다」는 위험을 주석으로 막고 있었던 셈이다.
// 이제 둘이 같은 함수를 탄다.
func ConvertStats(path string, dictPath string) (convert.Stats, error) {
	d, err := glossary.Load(dictPath)
	if err != nil {
		return convert.Stats{}, err
	}
	_, stats, err := loadAndConvert(path, d)
	return stats, err
}

// loadAndConvert는 «읽고 변환한다»까지다. 쓰기는 하지 않는다 —
// convertWithDict와 ConvertStats가 이 한 함수를 공유하는 것이 요점이다.
func loadAndConvert(path string, d *glossary.Dict) ([]byte, convert.Stats, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, convert.Stats{}, fmt.Errorf("read %q: %w", path, err)
	}
	out, stats, err := convert.File(src, d)
	if err != nil {
		return nil, convert.Stats{}, fmt.Errorf("convert %q: %w", path, err)
	}
	return out, stats, nil
}

func convertWithDict(path string, d *glossary.Dict, outDir string) (convert.Stats, string, bool, error) {
	out, stats, err := loadAndConvert(path, d)
	if err != nil {
		return convert.Stats{}, "", false, err
	}

	dst := convert.OutputPath(path, outDir)
	if err := guardNotOverwriting(path, dst); err != nil {
		return convert.Stats{}, "", false, err
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return convert.Stats{}, "", false, fmt.Errorf("create output dir: %w", err)
		}
	}
	// 쓰기 «전»에 봐야 한다. 쓰고 나서 보면 언제나 있다.
	// Stat이 다른 이유(권한 등)로 실패하면 "없었다"로 친다 — 그 경우
	// 이어지는 WriteFile이 같은 이유로 실패해 진짜 에러를 낸다.
	overwritten := false
	if _, err := os.Stat(dst); err == nil {
		overwritten = true
	}
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return convert.Stats{}, "", false, fmt.Errorf("write %q: %w", dst, err)
	}
	return stats, dst, overwritten, nil
}

// guardNotOverwriting은 출력이 입력을 덮어쓰지 못하게 막는다. 논리 ERD는
// 사람이 손으로 그린 원본이며, 잃으면 되돌릴 길이 없다.
func guardNotOverwriting(src, dst string) error {
	a, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("abs %q: %w", src, err)
	}
	b, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("abs %q: %w", dst, err)
	}
	if a == b {
		return fmt.Errorf("output %q would overwrite the input file", dst)
	}
	return nil
}

// ConvertFolder는 디렉터리 안의 .drawio를 전부 변환한다. 파일 목록 규칙은
// findDrawioFiles로 generate와 공유한다 — 두 경로가 다른 규칙으로 파일을
// 고르면 결과가 갈린다.
//
// 이미 변환된 산출물(*.physical.drawio)은 건너뛴다. 출력이 입력 옆에
// 놓이므로, 같은 폴더에 두 번 돌리면 1회차 산출물이 2회차 입력이 되어
// X.physical.physical.drawio가 생긴다. 파일 하나를 직접 지정한 Convert는
// 건너뛰지 않는다 — 사용자가 명시한 것이다.
//
// 사전은 한 번만 읽어 모든 파일에 재사용한다.
func ConvertFolder(dir string, cfg config.Config, dictPath, outDir string) ([]ConvertFileResult, error) {
	files, err := findDrawioFiles(dir, cfg.Recursive)
	if err != nil {
		return nil, err
	}
	d, err := glossary.Load(dictPath)
	if err != nil {
		return nil, err
	}

	results := make([]ConvertFileResult, 0, len(files))
	// dstOwner는 이 배치 안에서 어느 출력 경로를 어느 입력이 이미 차지했는지
	// 추적한다. convert.OutputPath는 outDir이 주어지면 원본이 어느 폴더에
	// 있었는지는 버리고 basename만 본다 — 재귀 스캔(cfg.Recursive)에서
	// 서로 다른 하위 폴더의 동명이인(a/주문.drawio, b/주문.drawio)이 같은
	// 출력 경로로 몰릴 수 있다. 막지 않으면 둘째 쓰기가 첫째를 조용히
	// 덮어쓰고도 두 결과 모두 성공(Err == nil)으로 돌아간다 — 사용자는 두
	// 파일이 변환됐다고 듣지만 하나는 사라진 뒤다.
	dstOwner := map[string]string{}
	for _, f := range files {
		if isPhysicalOutput(f) {
			continue
		}
		dst := filepath.Clean(convert.OutputPath(f, outDir))
		if owner, taken := dstOwner[dst]; taken {
			results = append(results, ConvertFileResult{
				SourceFile: f,
				Err:        fmt.Errorf("output %q would overwrite the result already written for %q in this batch", dst, owner),
			})
			continue
		}
		stats, out, overwritten, err := convertWithDict(f, d, outDir)
		if err != nil {
			results = append(results, ConvertFileResult{SourceFile: f, Err: err})
			continue
		}
		dstOwner[dst] = f
		results = append(results, ConvertFileResult{
			SourceFile:  f,
			OutputFile:  out,
			Stats:       stats,
			Overwritten: overwritten,
		})
	}
	return results, nil
}

// isPhysicalOutput은 path가 convert의 산출물 이름 규칙(X.physical.drawio)을
// 따르는지 본다. 확장자 대소문자는 findDrawioFiles와 같이 구분하지 않지만
// (X.physical.DRAWIO도 건너뛴다) 접미사 자체는 구분한다 — 우리가 쓰는
// 이름은 언제나 소문자 ".physical"이므로, 사람이 손으로 붙인
// X.PHYSICAL.drawio는 건너뛰지 않고 그냥 입력으로 본다.
func isPhysicalOutput(path string) bool {
	base := filepath.Base(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return strings.HasSuffix(stem, physicalStemSuffix)
}
