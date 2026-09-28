// readme_test.go — README.md의 내부 링크가 실제 제목을 가리키는지 본다.
//
// 이 파일이 있는 이유: README의 "한눈에 보기" 표는 각 커맨드의 상세 절로
// 곧바로 가는 앵커 링크를 든다. 그 앵커는 제목에서 «자동으로» 만들어지는
// 문자열이라(GitHub·VS Code 미리보기가 같은 규칙을 쓴다), 제목을 한 글자
// 고치면 링크가 **아무 경고 없이** 죽는다. 클릭해 보기 전에는 아무도 모르고,
// 클릭한 사람은 페이지 맨 위로 튕길 뿐 «깨졌다»는 신호를 못 받는다.
//
// 이 저장소가 반복해서 데인 실패 유형이 정확히 그것이다 — 문서가 조용히
// 거짓말을 시작하는 것. 그래서 시끄럽게 실패시킨다.
package erdtool

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// headingRE는 ##·### 제목을 잡는다. 코드 블록 안의 "#"은 아래에서 따로 뺀다.
var headingRE = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*$`)

// linkRE는 [글자](#앵커) 꼴의 문서 내부 링크를 잡는다.
var linkRE = regexp.MustCompile(`\[[^\]]*\]\(#([^)]+)\)`)

// punctRE는 github-slugger가 지우는 문자들이다 — 글자·숫자·결합기호·
// 밑줄·하이픈·공백만 남는다.
var punctRE = regexp.MustCompile(`[^\p{L}\p{N}\p{M}_\- ]`)

// slugify는 GitHub(github-slugger)와 VS Code 미리보기가 제목에서 앵커를
// 만드는 규칙이다: 소문자로 낮추고, 구두점을 지우고, 공백을 하이픈으로.
//
// 지우기가 «먼저»이고 공백 치환이 «나중»이라는 순서가 중요하다. 그래서
// " — "처럼 공백에 둘러싸인 구두점은 하이픈 두 개(`--`)를 남긴다 —
// 이 README의 제목들이 실제로 그 모양이다.
func slugify(heading string) string {
	s := strings.ToLower(strings.TrimSpace(heading))
	s = punctRE.ReplaceAllString(s, "")
	return strings.ReplaceAll(s, " ", "-")
}

// readmeHeadingsAndLinks는 README를 읽어 제목의 앵커 집합과 내부 링크
// 목록을 돌려준다. 코드 블록(```) 안은 건너뛴다 — 사용법 예시의 주석에
// "#"이 들어 있어 제목으로 오인되기 때문이다.
func readmeHeadingsAndLinks(t *testing.T) (map[string]bool, []string) {
	t.Helper()
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("README.md 읽기: %v", err)
	}

	anchors := map[string]bool{}
	var links []string
	inCode := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if m := headingRE.FindStringSubmatch(line); m != nil {
			anchors[slugify(m[1])] = true
			continue
		}
		for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
			links = append(links, m[1])
		}
	}
	return anchors, links
}

func TestREADMEInternalLinksResolve(t *testing.T) {
	anchors, links := readmeHeadingsAndLinks(t)

	if len(links) == 0 {
		t.Fatal("내부 링크가 하나도 없다 — 이 테스트가 지킬 것이 사라졌다면 함께 지워라")
	}
	for _, l := range links {
		if !anchors[l] {
			t.Errorf("링크 #%s가 가리키는 제목이 없다.\n"+
				"제목을 고쳤다면 링크도 함께 고쳐라. 실제 앵커 목록: %v", l, sortedKeys(anchors))
		}
	}
}

// 표가 커맨드를 빠뜨리면 «한눈에 보기»가 거짓이 된다. 서브커맨드 절이
// 새로 생겼는데 표에 안 실리면 여기서 잡는다.
func TestREADMESummaryTableCoversEverySubcommand(t *testing.T) {
	_, links := readmeHeadingsAndLinks(t)
	linked := map[string]bool{}
	for _, l := range links {
		linked[l] = true
	}

	anchors, _ := readmeHeadingsAndLinks(t)
	for a := range anchors {
		// 서브커맨드 절의 제목은 전부 커맨드 이름으로 시작한다.
		for _, cmd := range []string{"generate", "convert", "build", "reverse", "annotate", "watch", "version"} {
			if a == cmd || strings.HasPrefix(a, cmd+"-") {
				if !linked[a] {
					t.Errorf("서브커맨드 절 #%s가 «한눈에 보기» 표에 없다", a)
				}
			}
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
