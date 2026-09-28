# 실제 sakila.drawio의 셀을 <object>로 감싸 erd_* 속성을 붙인다.
# 손으로 .drawio를 적지 않기 위한 스크립트다(저장소 설계 원칙 3).
import re
import sys
from xml.sax.saxutils import quoteattr

src, dst = sys.argv[1], sys.argv[2]
s = open(src, encoding="utf-8").read()

# (셀 id) -> {속성이름: 값}
WRAP = {
    # 테이블 제약 — draw.io에서 «한 번 클릭»으로 잡히는 그 셀
    "p0_t5": {
        "erd_comment": "영화 마스터",
        "erd_check": "rental_duration >= 1",
    },
    # 컬럼 제약 — 정의 셀(권장 자리, «두 번 클릭»)
    "p0_t5_r0d": {"erd_comment": "영화 식별자"},
}

# 아래는 실수를 일부러 심는다. 검증이 잡는지 보기 위한 것이다.
# 1) 같은 컬럼(customer.email)에 같은 이름, 다른 값 — 키 셀과 정의 셀에 하나씩
WRAP["p0_t11_r4k"] = {"erd_check": "LENGTH(email) >= 5"}
WRAP["p0_t11_r4d"] = {"erd_check": "email LIKE '%@%'"}
# 2) 오타 — draw.io도 파서도 아무 말 없이 값을 버리는 자리
WRAP["p0_t11_r2d"] = {"erd_chek": "LENGTH(first_name) >= 1"}
# 3) 컬럼 전용 이름을 테이블에 붙였다
WRAP["p0_t0"] = {"erd_default": "'KR'"}


def wrap(m):
    cell_id = m.group("id")
    attrs = WRAP.get(cell_id)
    if attrs is None:
        return m.group(0)
    inner = m.group(0)
    # 안쪽 <mxCell>에서 id와 value를 떼어 <object>로 올린다 —
    # draw.io가 «데이터 편집»으로 저장할 때 만드는 모양 그대로다.
    value = ""
    vm = re.search(r'\svalue="([^"]*)"', inner)
    if vm:
        value = vm.group(1)
        inner = inner.replace(vm.group(0), "", 1)
    inner = re.sub(r'\sid="[^"]*"', "", inner, count=1)
    pairs = " ".join(
        "%s=%s" % (k, quoteattr(v)) for k, v in sorted(attrs.items())
    )
    return '<object label="%s" %s id="%s">%s</object>' % (value, pairs, cell_id, inner)


out, count = re.subn(
    r'<mxCell (?P<head>[^>]*?id="(?P<id>[^"]+)"[^>]*)>.*?</mxCell>',
    wrap,
    s,
    flags=re.S,
)
open(dst, "w", encoding="utf-8").write(out)

wrapped = out.count("<object ")
print("셀 %d개를 훑어 %d개를 감쌌다 (기대: %d)" % (count, wrapped, len(WRAP)))
if wrapped != len(WRAP):
    sys.exit("감싼 수가 기대와 다르다 — 셀 id를 확인하라")
