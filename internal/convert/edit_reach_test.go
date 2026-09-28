package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"erdtool/internal/drawio"
	"erdtool/internal/glossary"
)

// TestEditsReachEveryTargetCell은 «파서가 셀로 인정하는 모든 id에 편집이
// 닿는가»를 실제 픽스처 전부에 대해 검사한다.
//
// 왜 이 테스트가 있는가: 파서(internal/drawio/xmlraw.go)는 <object>와
// <UserObject>를 둘 다 «id를 가진 셀»로 평탄화하는데, 재작성기
// (internal/convert/rewrite.go)는 한때 <object>만 봤다. 그래서 <UserObject>로
// 감싸인 셀에 편집을 걸면 «에러 없이 아무 일도 일어나지 않았다».
//
// 두 파일이 그 요소 이름 목록을 각각 따로 들고 있어서, 어긋나면 같은 조용한
// 실패가 재발한다. buildEdits가 대상으로 삼은 id 전부가 재작성 뒤 실제로
// 값이 바뀌었는지 확인해서 그 어긋남을 여기서 잡는다.
func TestEditsReachEveryTargetCell(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "drawio", "testdata", "*.drawio"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("픽스처를 하나도 못 찾았다")
	}

	grandTotal := 0
	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			diagrams, err := drawio.LoadDiagramsBytes(src)
			if err != nil {
				t.Fatalf("LoadDiagramsBytes: %v", err)
			}
			perPage, err := collectLogicalNames(src)
			if err != nil {
				t.Fatalf("collectLogicalNames: %v", err)
			}
			// 이 테스트는 File이 아니라 collectLogicalNames와 RewriteMxFile을
			// 직접 쓴다. RewriteMxFile은 한 벌의 편집 맵을 모든 페이지에
			// 그대로 적용한다는 옛 뜻을 지키는 함수이므로(rewrite.go 참고),
			// 여기서도 페이지별 결과를 평탄화해 그 계약에 맞춘다 — File의
			// 페이지별 적용과는 다른 코드 경로를 검사하는 테스트다.
			logicalOf := map[string]string{}
			for _, m := range perPage {
				for k, v := range m {
					logicalOf[k] = v
				}
			}

			// 1차: 빈 사전으로 «무엇이 대상인지»와 그 논리명만 알아낸다.
			// 빈 사전은 아무것도 못 맞히므로 Physical == 논리명 그대로다.
			probe := glossary.MustLoadForTest(nil, nil)
			d := changingDict(t, diagrams, probe, logicalOf)

			// 2차: 반드시 값이 «바뀌는» 사전으로 진짜 편집을 만든다.
			// 미매칭이면 값이 그대로라 이 검사가 무의미해진다.
			allEdits := map[string]Edit{}
			for _, dg := range diagrams {
				edits, _ := buildEdits(dg.Cells, d, logicalOf)
				for k, v := range edits {
					allEdits[k] = v
				}
			}
			if len(allEdits) == 0 {
				t.Logf("대상 셀 0개 — 테이블이 없는 픽스처")
				return
			}

			before := valuesByID(diagrams)
			out, err := rewriteMxFile(src, allEdits)
			if err != nil {
				t.Fatalf("RewriteMxFile: %v", err)
			}
			rewritten, err := drawio.LoadDiagramsBytes(out)
			if err != nil {
				t.Fatalf("결과 재파싱: %v", err)
			}
			after := valuesByID(rewritten)

			// 저장된 논리명도 같은 요소 이름 목록에 걸린다 —
			// collectLogicalNames가 셋째 목록을 들고 있으므로 함께 본다.
			savedLogicalPerPage, err := collectLogicalNames(out)
			if err != nil {
				t.Fatalf("결과에서 collectLogicalNames: %v", err)
			}
			// 위와 같은 이유로 여기도 평탄화한다 — RewriteMxFile로 적용한
			// 결과이므로 페이지별로 가를 이유가 없다.
			savedLogical := map[string]string{}
			for _, m := range savedLogicalPerPage {
				for k, v := range m {
					savedLogical[k] = v
				}
			}

			for id, e := range allEdits {
				got, ok := after[id]
				if !ok {
					t.Errorf("id %q: 재작성 결과에 셀이 사라졌다", id)
					continue
				}
				if got != e.Label {
					t.Errorf("id %q: value = %q; want %q (파서는 아는데 재작성기가 못 건드린 셀)", id, got, e.Label)
					continue
				}
				if got == before[id] {
					t.Errorf("id %q: 값이 그대로다(%q) — 사전이 안 맞아 검사가 무의미하다", id, got)
				}
				if savedLogical[id] != e.LogicalName {
					t.Errorf("id %q: logicalName = %q; want %q", id, savedLogical[id], e.LogicalName)
				}
			}
			if !t.Failed() {
				t.Logf("대상 셀 %d개 전부 편집이 닿았다", len(allEdits))
			}
			grandTotal += len(allEdits)
		})
	}
	if grandTotal == 0 {
		t.Fatal("어느 픽스처에서도 대상 셀을 못 찾았다 — 테스트가 아무것도 검사하지 않았다")
	}
	t.Logf("픽스처 %d개, 대상 셀 합계 %d개", len(files), grandTotal)
}

// changingDict는 이 픽스처에 실제로 등장하는 논리명만 담은 사전을 만든다.
// 모든 논리명이 «원문과 다른» 물리명으로 바뀌도록 짠다.
func changingDict(t *testing.T, diagrams []drawio.RawDiagram, probe *glossary.Dict, logicalOf map[string]string) *glossary.Dict {
	t.Helper()
	terms := map[string]string{}
	n := 0
	for _, dg := range diagrams {
		_, stats := buildEdits(dg.Cells, probe, logicalOf)
		for _, det := range stats.Details {
			if _, ok := terms[det.Logical]; ok {
				continue
			}
			n++
			physical := fmt.Sprintf("PHYS%d", n)
			terms[det.Logical] = physical
			// 밑줄이 있는 이름은 Convert가 밑줄을 뗀 문자열로 용어사전을
			// 조회한다(RouteTermJoined). 그 키도 같은 값으로 넣어둔다.
			if joined := strings.ReplaceAll(det.Logical, "_", ""); joined != det.Logical {
				if _, ok := terms[joined]; !ok {
					terms[joined] = physical
				}
			}
		}
	}
	return glossary.MustLoadForTest(terms, nil)
}

func valuesByID(diagrams []drawio.RawDiagram) map[string]string {
	out := map[string]string{}
	for _, dg := range diagrams {
		for _, c := range dg.Cells {
			out[c.ID] = c.Value
		}
	}
	return out
}
