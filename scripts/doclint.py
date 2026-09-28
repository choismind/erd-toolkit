#!/usr/bin/env python3
"""문서에서 반복해 온 실수를 «고칠 수 있는 것만» 잡는다.

설계 전제: **전수 검사를 돌리지 않는다.** 훅이 방금 저장한 파일 하나만
검사하고 끝난다. 사람이 따로 돌릴 일도, 기다릴 일도 없어야 한다.

그래서 규칙은 오탐이 거의 없는 둘만 둔다.
  1. 지어낸 UI 용어 — 목록에 있는 낱말이 나오면 무조건 틀린 것이다
  2. 은어와 풀어 쓴 IT 용어 — 독자는 개발자다. 통로가 아니라 채널이다
  3. 증거 없이 쓰기 쉬운 단정 — 몇 개 안 되고, 나올 때마다 실제로 봐야 한다

문체 휴리스틱(문장 길이·「—」 개수)은 오탐이 많아 **기본으로 끈다.**
문서를 크게 손본 뒤 한 번씩 `--style`로만 본다.

    python scripts/doclint.py --hook      # 훅이 이렇게 부른다(stdin으로 JSON)
    python scripts/doclint.py <파일…>
    python scripts/doclint.py --changed   # git이 잡은 변경 .md만
    python scripts/doclint.py --changed --added-only   # 새로 쓴 줄만 (CI)
    python scripts/doclint.py --style <파일…>

위반이 있으면 종료 코드 1.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

# draw.io 화면에 없는 말. 백틱 안(코드 식별자 `addErPalette`)은 통과시킨다.
BANNED = {
    "팔레트": "UI를 가리키면 「도형」 패널 / 「ER」 카테고리",
    "사이드바": "「도형」 패널",
    "서랍": "비유다. 「ER」 카테고리",
    "머리칸": "「제목 줄」",
    "키 칸": "「키 셀」 — draw.io 공식 용어는 cell",
    "정의 칸": "「정의 셀」 — draw.io 공식 용어는 cell",
    # ERD의 대상은 DB 테이블이다. 「표」는 도구 모음 버튼 이름일 뿐,
    # 여기서 말하는 그것이 아니다. 오탐이 없는 어형만 잡는다.
    "표 전체": "「테이블 전체」",
    "표 제약": "「테이블 제약」",
    "표인가": "「테이블인가」 — ERD의 대상은 DB 테이블이다",
    "표 → 행": "「테이블 → 행」",
    "표도 행도": "「테이블도 행도」",
    "표·행·셀": "「테이블·행·셀」",
}

# 은어와 «풀어 쓴 IT 용어». 문서를 검토할 때마다 되풀이해 나온 것들이라
# 규칙으로 적어 두는 것만으로는 안 잡혔다. 코드블록 안(저장소 주석 인용)은
# 위 루프가 이미 건너뛰므로 실물과 어긋날 걱정은 없다. 오탐을 피하려고
# 어형이 하나뿐인 낱말만 넣는다.
JARGON = {
    "훑": "「차례로 읽는다」",
    "짚": "「진단을 낸다」·「가리킨다」",
    "돈다": "「실행된다」",
    "태우": "「넣는다」",
    "흘리": "「그대로 내보낸다」",
    "이름표": "「목록」·「이름」",
    "걸음": "「단계」",
    "껍데기": "「래퍼」",
    "통로": "「채널」 또는 「출력 스트림」",
    "갈래": "「분기」",
    "되풀이": "「반복」",
    "사는 자리": "「주소」",
}

# 증거와 대조하지 않고 쓰기 쉬운 단정. 지우라는 게 아니라, 바로 아래 증거와
# 한 줄씩 맞춰 봤는지 묻는 것이다. **git이 «새로 쓴 줄»이라 말한 줄에만
# 적용한다** — 이미 검토를 마친 문장에 매번 다시 뜨면 그게 곧 전수 검사가
# 되고, 아무도 안 읽게 된다. 저장소 밖의 파일에서는 아예 끈다.
ABSOLUTE = re.compile(r"통째로|하나도|빠짐없이|잘라내거나|전부 그대로|예외 없이")

# 용어 규칙을 «설명하는» 줄에는 금지 용어가 나올 수밖에 없다. 그 줄 끝에
# 이 표시를 달면 넘어간다.
ALLOW = "<!-- doclint-allow -->"

# ── 이 저장소의 도메인 검사 ────────────────────────────────────────────
# draw.io 스타일 토큰은 철자 하나가 틀리면 문서가 조용히 거짓이 된다.
# 실측으로 확정된 이름만 옳다고 본다(docs/drawio-er-palette.md).
STYLE_TOKENS = {
    "shape=table", "shape=tableRow", "shape=partialRectangle",
    "shape=ext", "shape=doubleEllipse", "shape=associativeEntity",
    "shape=rhombus", "shape=note", "shape=link", "shape=cloud",
    "edgeStyle=entityRelationEdgeStyle", "childLayout=tableLayout",
}
STYLE_RE = re.compile(r"(?<![A-Za-z])(?:shape|edgeStyle|childLayout)=([A-Za-z][A-Za-z0-9]*)")

# 카디널리티 화살표. C계열 16종을 만드는 부품은 이 여섯뿐이다.
ARROWS = {"ERone", "ERmandOne", "ERmany", "ERoneToMany", "ERzeroToOne", "ERzeroToMany"}
ARROW_RE = re.compile(r"(?<![A-Za-z])(?:endArrow|startArrow)=(ER[A-Za-z]{2,})")

# 문서가 가리키는 저장소 파일이 아직 있는지 본다. 경로(«/» 포함)만
# 본다 — 이미 지운 옛 문서를 이야기하는 문장까지 잡으면 오탐이 된다.
PATHISH = re.compile(r"`([A-Za-z0-9_./-]+\.(?:go|py|sh|drawio|xml|yaml|yml|json))`")

INLINE_CODE = re.compile(r"`[^`]*`")
FENCE = re.compile(r"^\s*```")

MAX_SENTENCE = 150
MAX_DASHES = 2


def repo_root() -> Path:
    return Path(__file__).resolve().parent.parent


def git_ignored(token: str) -> bool:
    """저장소 기준 경로 token을 git이 무시하면 True. 파일이 없어도 답한다."""
    r = subprocess.run(["git", "check-ignore", "-q", "--", token],
                       cwd=repo_root(), capture_output=True)
    return r.returncode == 0


def missing_ref(token: str, doc: Path) -> bool:
    """문서가 저장소 기준 경로로 가리킨 파일이 없으면 True.

    로컬과 CI가 같은 답을 내는 것만 본다. 그래서 둘을 보지 않는다.
    - 경로 없이 이름만 적은 파일(`sakila_broken.drawio`): 어디를 가리키는지
      정할 수 없다. 이 기계의 sandbox/에서 찾아 통과시키면 CI에서는 걸린다.
    - git이 무시하는 경로(sandbox/ 같은 것): 이 기계에만 있고 클론에는 없다.
    """
    if "/" not in token:
        return False
    if git_ignored(token):
        return False
    return not (repo_root() / token).exists()


def record_dirs() -> list[str]:
    """용어 규칙에서 뺄 기록 폴더를 `.git/info/doclint-records`에서 읽는다.

    커밋되지 않는 자리라 기계마다 따로 둔다. 한 줄에 폴더 하나이고 `#`로
    시작하는 줄은 건너뛴다. 파일이 없으면 뺄 것이 없다.
    """
    p = repo_root() / ".git" / "info" / "doclint-records"
    if not p.is_file():
        return []
    out = []
    for line in p.read_text(encoding="utf-8").splitlines():
        line = line.strip().strip("/")
        if line and not line.startswith("#"):
            out.append(line)
    return out


def is_record(path: Path) -> bool:
    try:
        rel = path.resolve().relative_to(repo_root()).as_posix()
    except ValueError:
        return False
    return any(rel == d or rel.startswith(d + "/") for d in record_dirs())


def check(path: Path, style: bool, added: set[int] | None = None,
          added_only: bool = False) -> list[str]:
    # 원장은 «그때 그렇게 판단했다»는 기록이다. 지난 항목의 낱말을 지금
    # 용어로 고쳐 쓰는 것은 기록을 손대는 것이므로 용어 규칙에서 뺀다.
    # 도메인 검사(스타일 토큰·화살표·파일 참조)는 그대로 적용한다.
    ledger = is_record(path)
    out: list[str] = []
    in_fence = False
    for no, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if FENCE.match(raw):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        if ALLOW in raw:
            continue
        line = INLINE_CODE.sub("", raw)

        # added_only면 «이번에 새로 쓴 줄»만 본다. CI가 그 모드로 돌린다 —
        # 지나온 문서의 은어까지 잡으면 빌드가 늘 빨갛고, 그러면 아무도
        # 이 검사를 안 읽게 된다.
        fresh = (not added_only) or (added is not None and no in added)

        for word, fix in BANNED.items():
            if not ledger and fresh and word in line:
                out.append(f"{path}:{no}: 금지 용어 「{word}」 → {fix}")
        for word, fix in JARGON.items():
            if not ledger and fresh and word in line:
                out.append(f"{path}:{no}: 은어·풀어 쓴 말 「{word}」 → {fix}")
        if added and no in added and ABSOLUTE.search(line):
            out.append(f"{path}:{no}: 단정 표현 — 바로 아래 증거와 한 줄씩 대조할 것")

        # 도메인 검사는 코드 표기(백틱·코드블록) 안까지 본다. 스타일
        # 문자열은 거기 적히기 때문이다.
        for m in STYLE_RE.finditer(raw):
            tok = m.group(0)
            if tok in STYLE_TOKENS:
                continue
            near = [t for t in STYLE_TOKENS if t.lower() == tok.lower()]
            if near:
                out.append(f"{path}:{no}: 스타일 토큰 철자 「{tok}」 → 「{near[0]}」")
        for m in ARROW_RE.finditer(raw):
            if m.group(1) not in ARROWS:
                out.append(
                    f"{path}:{no}: 카디널리티 화살표 「{m.group(1)}」는 실측 6종이 아니다 "
                    f"({', '.join(sorted(ARROWS))})"
                )
        for m in PATHISH.finditer(raw):
            if missing_ref(m.group(1), path):
                out.append(f"{path}:{no}: 가리킨 파일이 저장소에 없다 — 「{m.group(1)}」")

        if not style or line.lstrip().startswith(("|", ">")):
            continue
        for sent in re.split(r"(?<=다\.)\s+", line):
            plain = re.sub(r"\*\*|\[\[|\]\]|[*_]", "", sent).strip()
            if len(plain) > MAX_SENTENCE:
                out.append(f"{path}:{no}: 한 문장 {len(plain)}자 — 나눠 쓸 것")
            if sent.count("—") >= MAX_DASHES:
                out.append(f"{path}:{no}: 한 문장에 「—」 {sent.count('—')}개 — 목록으로 쓸 것")
    return out


def base_ref() -> str:
    """무엇과 견줄 것인가. 기본은 워킹 트리(HEAD)이고, CI는 DOCLINT_BASE로
    «이 변경의 출발점»을 준다 — PR이면 base 커밋, main 직접 커밋이면 HEAD~1."""
    return os.environ.get("DOCLINT_BASE", "HEAD")


def added_lines(path: Path) -> set[int] | None:
    """git이 «이번에 새로 쓴 줄»이라고 보는 줄 번호. 추적 밖이면 None."""
    try:
        diff = subprocess.run(
            ["git", "diff", "-U0", base_ref(), "--", str(path)],
            capture_output=True, text=True, check=True, cwd=path.parent,
        ).stdout
    except (OSError, subprocess.CalledProcessError):
        return None
    if not diff:
        return set()
    nos: set[int] = set()
    for m in re.finditer(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@", diff, re.M):
        start, count = int(m.group(1)), int(m.group(2) or 1)
        nos.update(range(start, start + count))
    return nos


def changed_docs() -> list[Path]:
    try:
        out = subprocess.run(
            ["git", "diff", "--name-only", "--diff-filter=d", base_ref(), "--", "*.md"],
            capture_output=True, text=True, check=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError):
        return []
    # 지운 파일은 뺀다(--diff-filter=d). 추적만 끊고 로컬에 남긴 파일을
    # 로컬에서는 보고 CI에서는 못 보는 어긋남을 막는다.
    return [Path(p) for p in out.split("\n") if p.strip() and Path(p).is_file()]


def hook() -> int:
    """PostToolUse 훅. stdin의 JSON에서 파일 하나를 꺼내 그것만 본다.

    위반이 있으면 종료 코드 2로 stderr에 낸다 — 훅을 부른 쪽에 되돌아가는
    경로다. 사용자의 편집을 막지는 않는다.
    """
    import json
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        return 0
    inp = payload.get("tool_input") or {}
    resp = payload.get("tool_response") or {}
    raw = resp.get("filePath") or inp.get("file_path") or ""
    if not raw.lower().endswith(".md"):
        return 0
    path = Path(raw)
    if not path.is_file():
        return 0
    problems = check(path, style=False, added=added_lines(path))
    if not problems:
        return 0
    print("doclint — 문서 규칙 위반:", file=sys.stderr)
    for line in problems:
        print("  " + line, file=sys.stderr)
    return 2


def main() -> int:
    if "--hook" in sys.argv[1:]:
        return hook()
    args = [a for a in sys.argv[1:]]
    style = "--style" in args
    added_only = "--added-only" in args
    args = [a for a in args if a not in ("--style", "--added-only")]

    if "--changed" in args:
        files = changed_docs()
    else:
        files = [Path(a) for a in args if a.lower().endswith(".md") and Path(a).is_file()]
    if not files:
        return 0

    problems: list[str] = []
    for f in files:
        problems.extend(check(f, style, added_lines(f), added_only))
    for line in problems:
        print(line)
    if problems:
        print(f"\n{len(problems)}건 — scripts/doclint.py")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
