package glossary

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// realDictPath는 실물 표준용어사전이다. 외부 드라이브에 있으므로 없으면
// 테스트를 건너뛴다 — 다른 기계에서 상시 깨지면 안 된다.
const realDictPath = `E:\devol\python\pyenv11\drawio\data\표준용어사전.xlsx`

func loadRealDict(t *testing.T) *Dict {
	t.Helper()
	if _, err := os.Stat(realDictPath); err != nil {
		t.Skipf("실물 사전이 없어 건너뛴다: %s", realDictPath)
	}
	d, err := Load(realDictPath)
	if err != nil {
		t.Fatalf("Load(%s): %v", realDictPath, err)
	}
	return d
}

// TestKnownPairs는 레퍼런스 구현(kiwipiepy)이 만든 실제 산출물과 같은 답을
// 내는지 검사한다.
func TestKnownPairs(t *testing.T) {
	d := loadRealDict(t)

	f, err := os.Open("testdata/known_pairs.txt")
	if err != nil {
		t.Fatalf("골든 파일 열기: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	checked := 0
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 2 {
			t.Fatalf("골든 파일 형식 오류(탭 하나로 구분해야 한다): %q", line)
		}
		logical, want := cols[0], cols[1]
		if got := d.Convert(logical).Physical; got != want {
			t.Errorf("Convert(%q) = %q; want %q", logical, got, want)
		}
		checked++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("골든 파일 읽기: %v", err)
	}
	if checked != 13 {
		t.Fatalf("골든 쌍 %d건을 읽었다; 13건이어야 한다", checked)
	}
}

// TestFullGlossaryBenchmark는 공통표준용어 전수를 «용어사전에 없다고 치고»
// 단어사전만으로 복원해 표준 약어와 대조한다.
//
// 공통표준용어는 정의상 공통표준단어의 조합이므로, 이 재현율이 곧 "사전
// 분해가 형태소 분석기를 대신할 수 있는가"의 답이다. 설계 시 실측한 기준선은
// 1693건 중 1692건(99.9%), 분해 실패 0건이다.
func TestFullGlossaryBenchmark(t *testing.T) {
	d := loadRealDict(t)

	total, ok := 0, 0
	var misses []string
	for term, want := range d.terms {
		total++
		parts := d.lookupParts(d.segment(term))
		if joinParts(parts, nil) == want {
			ok++
			continue
		}
		if len(misses) < 10 {
			misses = append(misses, term+" -> "+joinParts(parts, nil)+" (want "+want+")")
		}
	}

	t.Logf("전수 벤치마크: %d건 중 %d건 일치", total, ok)
	if total < 1600 {
		t.Fatalf("사전이 예상보다 작다(%d건) — 다른 파일을 읽었을 수 있다", total)
	}
	// 기준선은 1692/1693. 사전 판본이 바뀔 수 있으므로 비율로 건다.
	if ratio := float64(ok) / float64(total); ratio < 0.995 {
		t.Fatalf("재현율 %.3f가 기준선 0.995 아래다. 어긋난 예: %v", ratio, misses)
	}
}
