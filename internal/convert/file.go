// internal/convert/file.go
package convert

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"

	"erdtool/internal/drawio"
	"erdtool/internal/glossary"
)

// File은 논리 .drawio 바이트를 읽어 물리 .drawio 바이트를 만든다.
//
// 세 걸음이다:
//  1. 기존 파서로 셀을 읽어 무엇을 바꿀지 정한다
//  2. 이미 저장돼 있던 logicalName을 모은다(파서가 커스텀 속성을 읽지 않으므로 따로)
//  3. 토큰 재작성으로 그 결정을 적용한다
//
// 입력은 끝까지 바이트로만 다룬다. drawio.LoadDiagramsBytes가 있으므로
// 임시 파일에 썼다가 되읽을 이유가 없다 — 그 왕복은 변환마다 디스크를 두 번
// 오가고, 정리에 실패하면 조용한 쓰레기를 남긴다.
func File(src []byte, d *glossary.Dict) ([]byte, Stats, error) {
	diagrams, err := drawio.LoadDiagramsBytes(src)
	if err != nil {
		return nil, Stats{}, err
	}

	logicalOf, err := collectLogicalNames(src)
	if err != nil {
		return nil, Stats{}, err
	}
	// logicalOf는 CountDiagrams 기준으로 길이가 잡히고 diagrams는
	// LoadDiagramsBytes에서 온다. 둘 다 같은 <diagram> 순회 규칙을 따르므로
	// 구조적으로는 길이가 같아야 하지만, 그 전제가 깨지면 아래 for문이
	// logicalOf[i]에서 바로 패닉(index out of range)한다. 위치 결합을 쓰는
	// 자리마다 그 전제를 스스로 확인해야 나중에 어느 한쪽이 바뀌어도 조용한
	// 어긋남 대신 여기서 바로 걸린다.
	if len(logicalOf) != len(diagrams) {
		return nil, Stats{}, fmt.Errorf(
			"페이지별 논리명은 %d개인데 파싱된 다이어그램은 %d개다 — 두 순회가 어긋났다",
			len(logicalOf), len(diagrams))
	}

	// 편집은 페이지별로 정해지고, 이제 페이지별로 적용된다. 셀 id가 페이지
	// 사이에서 겹쳐도 한쪽 편집이 다른 쪽에 번지지 않으므로, 예전에 여기
	// 있던 «겹치면 멈춘다» 가드는 필요 없다. 그 가드는 「편집이 파일 전체에
	// 적용된다」는 전제 위에서만 뜻이 있었고, 그 전제가 사라졌다.
	pages := make([]PagePlan, len(diagrams))
	var stats Stats
	for i, dg := range diagrams {
		edits, s := buildEdits(dg.Cells, d, logicalOf[i])
		pages[i] = PagePlan{DiagramID: dg.ID, Edits: cellEditsFrom(edits)}
		stats.Total += s.Total
		stats.Converted += s.Converted
		stats.Details = append(stats.Details, s.Details...)
		for _, u := range s.Unmatched {
			if !containsString(stats.Unmatched, u) {
				stats.Unmatched = append(stats.Unmatched, u)
			}
		}
	}

	out, err := RewriteMxFilePlan(src, RewritePlan{Pages: pages})
	if err != nil {
		return nil, Stats{}, err
	}
	return out, stats, nil
}

// collectLogicalNames는 파일 전체에서 <object ... logicalName="...">를
// 페이지별로 모은다. 첨자가 <diagram>의 등장 순서다.
//
// drawio.LoadDiagrams는 <object>의 id와 label만 읽고 커스텀 속성은
// 버리므로, 멱등성에 필요한 논리명을 여기서 따로 읽는다.
//
// 페이지별로 가르는 이유: 셀 id가 페이지 사이에서 겹치면 파일 전체 맵
// 하나로는 뒤 페이지 값이 앞 페이지 값을 덮는다.
//
// 압축 판별·해제는 ForEachGraphModel이 한다 — 예전에는 이 함수가 같은
// 로직을 그대로 중복해서 갖고 있었다.
func collectLogicalNames(src []byte) ([]map[string]string, error) {
	n, err := CountDiagrams(src)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]string, n)
	for i := range out {
		out[i] = map[string]string{}
	}
	err = ForEachGraphModel(src, func(pageIdx int, _ string, graphModel []byte) error {
		return scanLogicalNames(graphModel, out[pageIdx])
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scanLogicalNames는 평문 mxGraphModel 하나에서 <object>/<UserObject>의
// logicalName 속성을 out에 채운다. collectLogicalNames의 옛 scan 클로저를
// 그대로 꺼낸 것이다.
func scanLogicalNames(graphModel []byte, out map[string]string) error {
	dec := xml.NewDecoder(bytes.NewReader(graphModel))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("scan logicalName: %w", err)
		}
		st, ok := tok.(xml.StartElement)
		// 파서가 <object>와 <UserObject>를 똑같이 «id를 가진 셀»로
		// 평탄화하므로 여기서도 둘 다 본다. 한쪽만 보면 그 셀은
		// 저장된 논리명이 없는 것처럼 보여 2회차에 label이 소스가 된다.
		if !ok || (st.Name.Local != "object" && st.Name.Local != "UserObject") {
			continue
		}
		id, hasID := attrValue(st.Attr, "id")
		logical, hasLogical := attrValue(st.Attr, logicalNameAttr)
		if hasID && hasLogical {
			out[id] = logical
		}
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// OutputPath는 입력 경로에서 출력 경로를 만든다.
//
//	주문.drawio -> 주문.physical.drawio
//
// outDir이 비어 있지 않으면 그 폴더에 같은 파일명으로 쓴다. 레퍼런스는
// 주문(physical).drawio를 썼지만 괄호는 셸에서 매번 인용이 필요하다.
func OutputPath(inputPath, outDir string) string {
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)] + ".physical" + ext
	if outDir != "" {
		return filepath.Join(outDir, name)
	}
	return filepath.Join(filepath.Dir(inputPath), name)
}
