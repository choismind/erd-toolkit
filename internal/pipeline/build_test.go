package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

const validCSV = `테이블명,순번,컬럼명,컬럼유형,색인여부,널허용,단일값
고객,1,고객번호,int,PK,False,False
## 관계
순번,원천테이블명,원천컬럼명,원천카디널리티,원천색인,연관명,목표카디널리티,목표색인,목표테이블명,목표컬럼명
`

func TestOutputPath_ReplacesExtensionWithDrawio(t *testing.T) {
	got := OutputPath(filepath.FromSlash("dir/설계서.xlsx"))
	want := filepath.FromSlash("dir/설계서.drawio")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuild_WritesDrawioFromCSV(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "설계서.csv")
	if err := os.WriteFile(in, []byte(validCSV), 0o644); err != nil {
		t.Fatalf("fixture 작성 실패: %v", err)
	}

	res, err := Build(in, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Overwritten {
		t.Fatal("첫 실행인데 Overwritten=true")
	}
	if res.Pages != 1 || res.Tables != 1 {
		t.Fatalf("got %+v", res)
	}
	if _, err := os.Stat(res.OutputPath); err != nil {
		t.Fatalf("출력 파일이 없다: %v", err)
	}
}

func TestBuild_SecondRunReportsOverwritten(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "설계서.csv")
	os.WriteFile(in, []byte(validCSV), 0o644)

	if _, err := Build(in, ""); err != nil {
		t.Fatalf("첫 실행 실패: %v", err)
	}
	res, err := Build(in, "")
	if err != nil {
		t.Fatalf("두번째 실행 실패: %v", err)
	}
	if !res.Overwritten {
		t.Fatal("두번째 실행은 Overwritten=true여야 한다")
	}
}

// build도 convert.go의 guardNotOverwriting과 같은 이유로 자기 입력을
// 지키지 못하면 안 된다: 데이터 설계서는 사람이 손으로 채운 원본이고,
// --out을 입력과 같은 경로로 주면(erdtool build 설계서.csv --out 설계서.csv)
// 되돌릴 길 없이 drawio XML로 덮어써진다(IMPORTANT 리뷰 발견).
func TestBuild_RefusesToOverwriteInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "설계서.csv")
	if err := os.WriteFile(in, []byte(validCSV), 0o644); err != nil {
		t.Fatalf("fixture 작성 실패: %v", err)
	}

	if _, err := Build(in, in); err == nil {
		t.Fatal("--out이 입력과 같은 경로면 에러여야 한다")
	}

	got, err := os.ReadFile(in)
	if err != nil {
		t.Fatalf("입력 파일이 사라졌다: %v", err)
	}
	if string(got) != validCSV {
		t.Fatal("입력 파일 내용이 바뀌었다 — 원본이 덮어써졌다")
	}
}

func TestBuild_UnknownExtensionIsError(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "설계서.txt")
	os.WriteFile(in, []byte(validCSV), 0o644)
	if _, err := Build(in, ""); err == nil {
		t.Fatal(".txt는 지원하지 않는 확장자이므로 에러여야 한다")
	}
}
