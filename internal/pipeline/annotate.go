// internal/pipeline/annotate.go
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"erdtool/internal/annotate"
	"erdtool/internal/config"
	"erdtool/internal/ddlcheck"
	"erdtool/internal/dialect"
	"erdtool/internal/ir"
	"erdtool/internal/report"
	"erdtool/internal/validate"
)

// AnnotateResult는 파일 하나의 annotate 처리 결과다.
type AnnotateResult struct {
	SourceFile string
	Marked     int
	Findings   int
	Pages      int
	Changed    bool
	Err        error

	// Warnings는 annotate.Result.Warnings를 그대로 나른다 — «멈추지는
	// 않지만 사용자가 반드시 알아야 하는» 사실들이고, CLI가 결과 줄
	// 밑에 찍는다. 여기서 삼키면 그 사실은 아무 데도 안 남는다.
	Warnings []string
}

// Annotate는 .drawio 하나를 «제자리에서» 표시한다.
//
// 다른 서브커맨드와 달리 새 파일을 만들지 않는다. 되반영의 쓸모가
// 거기 걸려 있다 — 진단을 보고 고칠 파일과 진단이 적힌 파일이 다르면
// 창을 둘 띄우고 오가게 된다.
//
// 제자리 수정의 위험은 두 갈래다. 하나는 "원치 않는데 맞게 써버림" —
// --dry-run이 미리 보여주고, --clean이 진짜 역연산이며(Task 13),
// 바뀐 것이 없으면 아예 쓰지 않는 것으로 막는다. 다른 하나는 "쓰다가
// 실패함" — 프로세스가 죽거나 디스크가 꽉 차거나 네트워크 공유가
// 끊기면, os.WriteFile은 O_TRUNC로 열어 원본을 먼저 비우기 때문에
// 실패 시점에 사용자 파일이 빈 채로 또는 잘린 채로 남는다. 위 세
// 방어는 이 경우를 못 막는다 — 그래서 실제 쓰기는 writeFileAtomic이
// 한다: 같은 디렉터리에 임시 파일을 쓰고 rename으로 덮어씌운다.
// rename은 같은 볼륨 안에서 원자적이라, rename 전에 실패하면 원본은
// 바이트 하나 안 바뀐 채로 남는다.
func Annotate(path string, cfg config.Config, clean, dryRun bool) (AnnotateResult, error) {
	res := AnnotateResult{SourceFile: path}

	src, err := os.ReadFile(path)
	if err != nil {
		return res, fmt.Errorf("read %q: %w", path, err)
	}

	var out []byte
	var r annotate.Result
	if clean {
		// --clean은 표식을 지우는 것뿐이라 원본 IR/검증이 필요 없다 —
		// annotate.Clean은 바이트만 보고 판단한다.
		out, r, err = annotate.Clean(src)
	} else {
		doc, dups, aerr := ir.Assemble(path)
		if aerr != nil {
			return res, aerr
		}
		// 타깃 DBMS는 generate와 같은 자리에서 푼다 — 모르는 값이면 파일을
		// 건드리기 전에 멈춘다. annotate에는 --dialect가 없으므로 설정
		// 파일의 dialect: 만 여기 들어온다.
		target, derr := dialect.Parse(cfg.Dialect)
		if derr != nil {
			return res, derr
		}
		findings := append(validate.Required(doc, dups), validate.Optional(doc, cfg)...)
		// 시험 실행(dry run)까지 돌려야 generate의 리포트와 «같은 건수»가 된다.
		// 이것이 없던 동안에는 ddl_syntax만 뜬 파일에서 리포트가 1건이라는데
		// annotate는 「진단 0건」이라 말했고, 사용자가 그 차이를 알 방법이
		// 없었다(2026-09-22 소유자 결정으로 이었다).
		//
		// 앞의 둘이 끝난 뒤에 도는 순서도 generate와 같다 — 구조가 이미
		// 진단이 난 테이블은 건너뛰어 같은 실수가 두 번 마크되지 않는다.
		opt := report.Options{PageAsDomain: cfg.PageAsDomain, Dialect: target}
		findings = append(findings, ddlcheck.DryRun(doc, findings, opt)...)
		out, r, err = annotate.Annotate(src, doc, findings)
	}
	if err != nil {
		return res, fmt.Errorf("%s: %w", path, err)
	}

	res.Marked, res.Findings, res.Pages, res.Changed = r.Marked, r.Findings, r.Pages, r.Changed
	res.Warnings = r.Warnings

	// 바뀐 것이 없으면 쓰지 않는다. 같은 바이트를 다시 써도 내용은
	// 같지만 mtime은 바뀌고, 그것이 watch나 빌드 도구를 깨운다 —
	// 아무 일도 안 한 사용자에게 "뭔가 바뀌었다"는 거짓 신호를 준다.
	// --dry-run도 같은 이유로 여기서 걸린다: 셈은 위에서 이미 다 했고
	// (res.Marked 등), 쓰기만 건너뛴다.
	if dryRun || !r.Changed {
		return res, nil
	}
	// 원본 파일의 권한 비트를 그대로 물려준다. 이미 위에서 os.ReadFile로
	// 읽는 데 성공했으므로 stat도 정상적으로 되는 게 보통이지만, 그
	// 사이 파일이 사라지는 등 실패하면 0o644로 물러난다 — 새로 만드는
	// 파일이 아니니 이 값은 거의 쓰일 일이 없다.
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode().Perm()
	}
	if err := writeFileAtomic(path, out, perm); err != nil {
		return res, fmt.Errorf("write %q: %w", path, err)
	}
	return res, nil
}

// writeFileAtomic은 path와 같은 디렉터리에 임시 파일을 만들어 data를
// 쓰고 Sync·Close한 뒤 path로 rename한다. rename은 실패하기 전까지
// 원본을 전혀 건드리지 않으므로, 쓰다가 죽거나 디스크가 꽉 차도
// 사용자 파일은 원래 상태 그대로 남는다 — os.WriteFile의 O_TRUNC와
// 다른 점이 이것이다. 실패 경로마다 임시 파일을 지운다.
//
// Windows에서 os.Rename은 대상이 이미 있는 일반 파일이면 그 자리에서
// 바꿔치기한다(MoveFileEx + MOVEFILE_REPLACE_EXISTING) — annotate_test.go의
// TestWriteFileAtomicReplacesContentAndLeavesNoTemp가 이걸 실제로
// 확인한다.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".erdtool-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // rename이 성공하면 이미 없어 no-op이다

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// os.CreateTemp는 0o600으로 만든다 — 원본 권한 비트를 물려준다.
	// Windows에서 os.Chmod는 유닉스 권한 비트를 그대로 흉내내지 못하고
	// 0o200(쓰기 가능 여부)만 읽기전용 속성에 반영한다; 최선을 다하는
	// 수준이고 여기서 실패해도 치명적이지 않으니 무시한다.
	_ = os.Chmod(tmpPath, perm)
	return os.Rename(tmpPath, path)
}

// AnnotateFolder는 디렉터리 안의 .drawio를 전부 처리한다. 파일 목록
// 규칙은 findDrawioFiles로 generate/convert와 공유한다 — 대소문자
// 구분 없는 확장자 비교, 비재귀 기본, cfg.Recursive일 때만 하위 폴더,
// generate의 산출물 폴더(reportDirSuffix) 제외까지 전부 그대로다.
//
// convert가 자기 산출물(*.physical.drawio)을 거르는 것과 달리 여기서는
// 거를 것이 없다 — annotate는 새 파일을 만들지 않으므로 findDrawioFiles가
// 찾아낸 목록이 곧 처리 대상 전부다.
//
// 파일 하나의 실패가 나머지를 막지 않는다(generate와 같은 파일 단위
// 에러 격리): 실패한 파일은 그 결과의 Err에 담기고 나머지는 계속
// 처리된다.
func AnnotateFolder(dir string, cfg config.Config, clean bool) ([]AnnotateResult, error) {
	paths, err := findDrawioFiles(dir, cfg.Recursive)
	if err != nil {
		return nil, err
	}
	results := make([]AnnotateResult, 0, len(paths))
	for _, path := range paths {
		r, aerr := Annotate(path, cfg, clean, false)
		if aerr != nil {
			r.Err = aerr
		}
		results = append(results, r)
	}
	return results, nil
}
