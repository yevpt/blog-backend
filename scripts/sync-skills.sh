#!/bin/sh
# 同步 skill 符号链接：为 .agents/skills/ 下每个 skill 在 .claude/skills/ 建立
# 相对符号链接（缺失才建，幂等）。新增 skill 后执行一次即可。
set -e
SRC=".agents/skills"
DST=".claude/skills"

[ -d "$SRC" ] || exit 0
mkdir -p "$DST"

created=0
for dir in "$SRC"/*/; do
  [ -d "$dir" ] || continue
  name=$(basename "$dir")
  link="$DST/$name"
  # 已存在（链接或目录，含失效链接）则跳过
  if [ -e "$link" ] || [ -L "$link" ]; then continue; fi
  ln -s "../../$SRC/$name" "$link"
  echo "linked skill: $name"
  created=$((created + 1))
done
[ "$created" -gt 0 ] && echo "synced $created skill(s)" || echo "skills up to date"
