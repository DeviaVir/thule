#!/usr/bin/env bash
set -euo pipefail

threshold="${1:-90}"
profile="${2:-unit.out}"

if [[ ! -f "$profile" ]]; then
  echo "coverage profile not found: $profile"
  exit 1
fi

total_line=$(go tool cover -func="$profile" | tail -n 1)
total_pct=$(echo "$total_line" | awk '{print $3}' | tr -d '%')

python3 - "$total_pct" "$threshold" <<'PY'
import sys
pct = float(sys.argv[1])
threshold = float(sys.argv[2])
if pct + 1e-9 < threshold:
    print(f"Coverage gate failed: total={pct:.1f}% threshold={threshold:.1f}%")
    sys.exit(1)
print(f"Coverage gate passed: total={pct:.1f}% threshold={threshold:.1f}%")
PY
