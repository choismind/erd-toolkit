package drawio

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
)

// RawGeometry는 셀의 mxGeometry다. 값이 없는 속성은 0이며, draw.io도
// 생략을 0으로 읽으므로 이 표현이 원본과 같은 뜻이다.
type RawGeometry struct {
	X      float64 `xml:"x,attr"`
	Y      float64 `xml:"y,attr"`
	Width  float64 `xml:"width,attr"`
	Height float64 `xml:"height,attr"`
	// Points는 mxGeometry의 직속 <mxPoint> 자식이다. 관계선의 끝이 어느
	// 도형에도 붙지 않았을 때 draw.io가 그 끝의 좌표를 여기에 남긴다.
	// 경유점(<Array as="points"> 안의 점)은 한 겹 더 안에 있어 여기
	// 들어오지 않는다.
	//
	// 이 필드가 생기면서 RawGeometry는 ==로 견줄 수 없게 됐다. 같은
	// 요소를 두 필드가 맡게 하는 방법(mxGeometry와 mxGeometry>mxPoint를
	// 함께 두는 것)은 encoding/xml이 실행 시점에 거부한다 — 빌드는
	// 통과하고 파일을 읽을 때 터진다.
	Points []RawPoint `xml:"mxPoint"`
}

// RawPoint는 mxGeometry 밑의 <mxPoint>다. As가 그 점이 무엇인지 말한다 —
// "sourcePoint"·"targetPoint"가 관계선의 두 끝이고, "offset"은 선에 적힌
// 글자의 위치다.
type RawPoint struct {
	X  float64 `xml:"x,attr"`
	Y  float64 `xml:"y,attr"`
	As string  `xml:"as,attr"`
}

type RawCell struct {
	ID     string `xml:"id,attr"`
	Value  string `xml:"value,attr"`
	Style  string `xml:"style,attr"`
	Parent string `xml:"parent,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
	// Geometry는 mxGeometry 자식이다. 없으면 nil이다 — 0으로 채운
	// 구조체와 «기하가 없다»를 구별해야 bounding box 계산이 원점에
	// 유령 사각형을 넣지 않는다.
	Geometry *RawGeometry `xml:"mxGeometry"`

	// Attrs는 <object>/<UserObject> 래퍼에 붙은 사용자 정의 이름/값 쌍이다
	// (draw.io의 Ctrl+M «데이터 편집»). 래퍼가 없는 순수 <mxCell>이면 nil이다.
	//
	// 구조 속성(id, label)은 여기 들어오지 않는다 — 그 둘은 이미 ID/Value로
	// 승격돼 있어서, 남겨 두면 같은 값이 두 자리에 생긴다. 그 밖의 것은
	// erd_ 접두어가 아니어도 **버리지 않는다.** 사용자들은 이 기능을 이미
	// 자기 용도로 쓰고 있고, 무엇이 붙어 있는지 알아야 «모르는 erd_* 이름»
	// (오타)을 진단으로 올릴 수 있기 때문이다.
	Attrs map[string]string
}

// rawObjectWrapper는 draw.io가 셀에 커스텀 데이터/플레이스홀더/링크를 붙일 때
// 쓰는 <object>(구버전) 또는 <UserObject>(신버전) 래퍼다. 이 경우 id와
// 표시값(label)은 래퍼 요소에 있고, 실제 도형 정보(style/parent/source/
// target)는 그 안의 <mxCell> 자식에 있다 — 반으로 쪼개져 있는 셈이라, 이걸
// 하나로 합치지 않으면 rawRoot가 이 셀을 통째로 못 본다(id가 없는 빈
// mxCell만 보임) — golden fixture entity_table_basic.drawio 3페이지의
// "도메인-1" 앵커 도형이 실제로 이 구조다.
type rawObjectWrapper struct {
	ID    string  `xml:"id,attr"`
	Label string  `xml:"label,attr"`
	Cell  RawCell `xml:"mxCell"`
	// Attrs는 위 둘을 포함한 «모든» 속성이다. encoding/xml의 ",any,attr"는
	// 이름을 지정한 필드가 이미 가져간 것도 다시 담으므로, id/label은
	// 아래 flatten에서 걸러낸다.
	Attrs []xml.Attr `xml:",any,attr"`
}

// structuralObjectAttrs는 <object> 래퍼에서 RawCell의 고유 필드로 승격되는
// 속성이다. 사용자 데이터로 다시 세지 않는다.
var structuralObjectAttrs = map[string]bool{"id": true, "label": true}

// flatten은 <object> 래퍼와 그 안의 <mxCell>을 RawCell 하나로 합친다.
func (w rawObjectWrapper) flatten() RawCell {
	cell := w.Cell
	cell.ID = w.ID
	cell.Value = w.Label
	for _, a := range w.Attrs {
		if structuralObjectAttrs[a.Name.Local] {
			continue
		}
		if cell.Attrs == nil {
			cell.Attrs = make(map[string]string)
		}
		cell.Attrs[a.Name.Local] = a.Value
	}
	return cell
}

type rawRoot struct {
	Cells []RawCell
}

// UnmarshalXML은 <root>의 자식을 순서대로 읽으며 <mxCell>은 그대로, <object>/
// <UserObject>는 평탄화해서 같은 []RawCell로 모은다. encoding/xml의 선언적
// 태그만으로는 서로 다른 두 요소 타입을 하나의 슬라이스로 못 모으므로 직접
// 토큰을 순회한다.
func (r *rawRoot) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "mxCell":
				var c RawCell
				if err := d.DecodeElement(&c, &t); err != nil {
					return err
				}
				r.Cells = append(r.Cells, c)
			case "object", "UserObject":
				var w rawObjectWrapper
				if err := d.DecodeElement(&w, &t); err != nil {
					return err
				}
				r.Cells = append(r.Cells, w.flatten())
			default:
				if err := d.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
}

type rawGraphModel struct {
	Root rawRoot `xml:"root"`
}

type rawDiagramXML struct {
	Name       string        `xml:"name,attr"`
	ID         string        `xml:"id,attr"`
	Text       string        `xml:",chardata"`
	GraphModel rawGraphModel `xml:"mxGraphModel"`
}

type rawMxFile struct {
	Diagrams []rawDiagramXML `xml:"diagram"`
}

type RawDiagram struct {
	ID    string
	Name  string
	Cells []RawCell
}

// LoadDiagrams는 .drawio/.xml 파일을 읽어 페이지(다이어그램) 목록을 반환한다.
// 압축/비압축, 단일/다중 페이지를 모두 지원한다.
func LoadDiagrams(path string) ([]RawDiagram, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return LoadDiagramsBytes(data)
}

// LoadDiagramsBytes는 이미 메모리에 있는 .drawio 바이트를 파싱한다.
//
// 파일에서 읽는 경로와 본문이 같으므로 LoadDiagrams가 이걸 부른다. 따로
// 두는 이유는, 바이트를 이미 손에 쥔 호출자(internal/convert)가 그것을 임시
// 파일에 썼다가 다시 읽는 왕복을 하지 않게 하기 위해서다 — 그 왕복은
// 변환 경로마다 디스크를 두 번 오가고, 임시 파일 정리에 실패하면 조용한
// 쓰레기를 남긴다.
func LoadDiagramsBytes(data []byte) ([]RawDiagram, error) {
	var mxfile rawMxFile
	if err := xml.Unmarshal(data, &mxfile); err != nil {
		return nil, fmt.Errorf("parse mxfile: %w", err)
	}

	diagrams := make([]RawDiagram, 0, len(mxfile.Diagrams))
	for _, d := range mxfile.Diagrams {
		cells := d.GraphModel.Root.Cells
		if len(cells) == 0 && d.Text != "" {
			// compressed="true" 인 경우 diagram 요소의 텍스트가 압축된
			// mxGraphModel XML이다.
			decoded, err := Decompress(d.Text)
			if err != nil {
				return nil, fmt.Errorf("decompress diagram %q: %w", d.Name, err)
			}
			var inner rawGraphModel
			if err := xml.Unmarshal([]byte(decoded), &inner); err != nil {
				return nil, fmt.Errorf("parse decompressed diagram %q: %w", d.Name, err)
			}
			cells = inner.Root.Cells
		}
		if len(cells) == 0 {
			// 정상적인 draw.io 페이지는 압축이든 아니든 최소한 구조상 필수
			// 셀(id="0" 루트, id="1" 기본 부모)을 반드시 담고 있다 — 진짜로
			// 빈 다이어그램이란 있을 수 없다. 0개 셀은 "빈 페이지"가 아니라
			// 파싱/포맷 불일치를 뜻하므로, 빈 테이블 목록을 성공(exit 0)으로
			// 조용히 흘려보내지 않고 여기서 바로 에러로 드러낸다.
			return nil, fmt.Errorf("diagram %q parsed to zero cells — likely a parse/format mismatch, not a legitimately empty diagram", d.Name)
		}
		diagrams = append(diagrams, RawDiagram{ID: d.ID, Name: d.Name, Cells: cells})
	}
	return diagrams, nil
}
