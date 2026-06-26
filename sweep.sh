#!/usr/bin/env bash
# Publication sweep for Track A, run via Bash for reliable raw-byte streaming
# (the PowerShell variant deadlocked in background and mangled encodings).
#
# Three phases merged into one benchstat input; cow's O(keys) write path is
# measured with a small FIXED iteration count (it is ~10^6x slower than the
# others, so a handful of samples already pin the number) while everything else
# gets precise time-based measurement. Output streams to results/bench.txt live,
# so progress is observable with `tail -f`.
set -uo pipefail

GO="go"
BENCHSTAT="$HOME/go/bin/benchstat"

KEYS="${KEYS:-1000000}"
KEYLEN="${KEYLEN:-16}"
COUNT="${COUNT:-10}"
CPU="${CPU:-1,2,4,8,12,16,20,24}"
BT="${BT:-1s}"
COWBT="${COWBT:-20x}"
OUT="${OUT:-results}"

mkdir -p "$OUT"
RAW="$OUT/bench.txt"
: > "$RAW"   # truncate

common=(-benchmem -count="$COUNT" -cpu="$CPU" -keys="$KEYS" -keylen="$KEYLEN" -run '^$')

echo "### phase A: fast impls (mutex|rwmutex|syncmap|sharded,syncXmap,otter), -benchtime=$BT"
"$GO" test -bench 'BenchmarkCache/impl=(mutex|rwmutex|syncmap|sharded|syncXmap|otter)' \
  "${common[@]}" -benchtime="$BT" | tee -a "$RAW"

# Anchor mix with $ -- "r10" is a substring of "r100".
echo "### phase B: cow reads (mix=r100), -benchtime=$BT"
"$GO" test -bench 'BenchmarkCache/impl=cow/dist=(uniform|zipf)/mix=r100$' \
  "${common[@]}" -benchtime="$BT" | grep -E '^Benchmark' | tee -a "$RAW"

echo "### phase C: cow writes (mix=r10|r50|r90), -benchtime=$COWBT"
"$GO" test -bench 'BenchmarkCache/impl=cow/dist=(uniform|zipf)/mix=(r10|r50|r90)$' \
  "${common[@]}" -benchtime="$COWBT" | grep -E '^Benchmark' | tee -a "$RAW"

echo "=== summary (mean +/- CV) -> $OUT/summary.txt ==="
"$BENCHSTAT" "$RAW" | tee "$OUT/summary.txt"

echo "=== comparison (impl as columns) -> $OUT/by-impl.txt ==="
# MSYS_NO_PATHCONV: stop Git Bash from rewriting the "/impl" projection arg
# into a Windows path (C:/Program Files/Git/impl), which benchstat rejects.
MSYS_NO_PATHCONV=1 "$BENCHSTAT" -col /impl "$RAW" | tee "$OUT/by-impl.txt"

echo "DONE"
