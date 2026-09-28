# internal/drawio/testdata/erd_attrs.drawio를 만든다.
#
# 실제 draw.io 저장본(relationship_physical.drawio)의 셀을 <object>로 감싸는
# 방식이며, 그것은 draw.io가 «데이터 편집»(Ctrl+M)으로 저장할 때 내는 모양과
# 같다. .drawio를 손으로 적지 않기 위한 것이다(설계 원칙 3).
import re
import sys
from xml.sax.saxutils import quoteattr

src, dst = sys.argv[1], sys.argv[2]
s = open(src, encoding="utf-8").read()

P = "XPPfe8yYSQ3rjpWAL3dI-"
WRAP = {
    # 테이블 제약 (CUST)
    P + "1": {"erd_comment": "고객", "erd_check": "CUST_NO > 0"},
    # 같은 컬럼(CUST_NO)의 키 셀과 정의 셀에 같은 이름, 다른 값 -> 충돌
    P + "3": {"erd_check": "CUST_NO > 0"},
    P + "4": {"erd_check": "CUST_NO >= 1"},
    # 정의 셀에 붙인 정상적인 컬럼 제약 (CUST_NM)
    P + "7": {"erd_default": "'미상'", "erd_comment": "고객명"},
    # 행 셀에 붙여도 그 컬럼의 것으로 받는다 (ORDR_YMD)
    P + "21": {"erd_comment": "주문일자"},
    # 오타 — 조용히 사라지면 안 되는 자리
    P + "23": {"erd_chek": "ORDR_YMD >= DATE '2000-01-01'"},
    # 컬럼 전용 이름을 테이블에 붙였다 (ORDR)
    P + "14": {"erd_default": "0"},
}


def wrap(m):
    cell_id = m.group("id")
    attrs = WRAP.get(cell_id)
    if attrs is None:
        return m.group(0)
    inner = m.group(0)
    value = ""
    vm = re.search(r'\svalue="([^"]*)"', inner)
    if vm:
        value = vm.group(1)
        inner = inner.replace(vm.group(0), "", 1)
    inner = re.sub(r'\sid="[^"]*"', "", inner, count=1)
    pairs = " ".join("%s=%s" % (k, quoteattr(v)) for k, v in sorted(attrs.items()))
    return '<object label="%s" %s id="%s">%s</object>' % (value, pairs, cell_id, inner)


out, scanned = re.subn(
    r'<mxCell (?P<head>[^>]*?id="(?P<id>[^"]+)"[^>]*)>.*?</mxCell>',
    wrap,
    s,
    flags=re.S,
)
wrapped = out.count("<object ")
print("셀 %d개를 훑어 %d개를 감쌌다 (기대: %d)" % (scanned, wrapped, len(WRAP)))
if wrapped != len(WRAP):
    sys.exit("감싼 수가 기대와 다르다 — 셀 id를 확인하라")
if "erdtool" in out:
    sys.exit("픽스처에 erdtool 문자열이 들어갔다 — 잔재 검사가 위양성을 낸다")
open(dst, "w", encoding="utf-8").write(out)
