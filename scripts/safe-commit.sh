#!/usr/bin/env bash
set -euo pipefail

# Safe atomic git commit for parallel Claude Code instances.
# Usage: ./scripts/safe-commit.sh "feat(stage-X): message" path1 path2 ...

if [ "$#" -lt 2 ]; then
  echo "Usage: $0 <commit-message> <path1> [path2 ...]" >&2
  exit 1
fi

COMMIT_MSG="$1"
shift
PATHS=("$@")

LOCK_FILE="/tmp/apexstock-git.lock"

(
  flock -w 120 200 || { echo "ERROR: Could not acquire git lock after 120s" >&2; exit 1; }

  EXISTING_PATHS=()
  for p in "${PATHS[@]}"; do
    if [ -e "$p" ] || git ls-files --error-unmatch "$p" >/dev/null 2>&1; then
      EXISTING_PATHS+=("$p")
    fi
  done

  if [ "${#EXISTING_PATHS[@]}" -eq 0 ]; then
    echo "No matching paths to commit."
    exit 0
  fi

  git add -A -- "${EXISTING_PATHS[@]}"
  if git diff --cached --quiet -- "${EXISTING_PATHS[@]}"; then
    echo "No staged changes in specified paths."
    exit 0
  fi

  git commit -m "$COMMIT_MSG" -- "${EXISTING_PATHS[@]}"
  echo "COMMITTED: $(git rev-parse --short HEAD) - $COMMIT_MSG"
) 200>"$LOCK_FILE"
