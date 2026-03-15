#!/usr/bin/env bash
set -euo pipefail

# Resolve the directory where this script lives.
SELF_PATH="${BASH_SOURCE[0]}"
while [ -h "$SELF_PATH" ]; do
  DIR="$(cd -P "$(dirname "$SELF_PATH")" && pwd)"
  SELF_PATH="$(readlink "$SELF_PATH")"
  [[ "$SELF_PATH" != /* ]] && SELF_PATH="$DIR/$SELF_PATH"
done
SELF_DIR="$(cd -P "$(dirname "$SELF_PATH")" && pwd)"

usage() {
  cat <<'EOF'
Run all go/re3 benchmarks (optionally filtered) and write one CSV per benchmark.

Outputs are written to:
  <script_dir>/../benchmarks/*.csv

All `rebar` commands are executed with the `rebar/` repository as the
current working directory.

USAGE
  run_benchmarks.sh [options]

OPTIONS
  -h, --help
      Show this help text.

  -o, --outdir DIR
      Output directory for per-benchmark CSV files.
      Default: <script_dir>/../benchmarks
      Env: OUTDIR

  -e, --engines "ENGINE1|ENGINE2|..."
      Engines passed to `rebar measure -e`.
      Default: go/regexp|go/re3
      Env: ENGINES

  -f, --filter REGEX
      Pass through to rebar to filter which benchmarks are listed/run.
      Env: FILTER

  --rebar DIR
      Path to the `rebar` repository root (its working directory).
      Default: sibling "rebar" repo next to this "re3" repo
      Env: REBAR

  --enable-metrics
      Enable OpenTelemetry (metrics, spans, logging) and write to metrics.log
      next to the benchmark CSV. Sets OTEL_ENABLED=true and OTEL_FILE_PATH.
      Env: ENABLE_METRICS (set to 1 or non-empty)

  -t, --timeout DURATION
      Per-benchmark timeout passed to `rebar measure --timeout`.
      Default: 10s. Env: TIMEOUT

NOTES
  - If the `rebar` binary is missing under the repo's `target/` directory,
    the script will exit with a message showing the `cargo build` command
    needed to create it.
EOF
}

# Defaults (may be overridden by env vars and/or flags below).
# Default OUTDIR is a simple UTC timestamped directory:
#   <script_dir>/../benchmarks/YYYY/MM/DD/HH/MM
DEFAULT_OUTDIR="${SELF_DIR}/../benchmarks/$(date -u +'%Y/%m/%d/%H/%M')"
RUN_AT_UTC="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
OUTDIR="${OUTDIR:-$DEFAULT_OUTDIR}"
FILTER="${FILTER:-}"
ENGINES="${ENGINES:-go/regexp|go/re3}"
SEARCH_ROOT="${REBAR:-}"
ENABLE_METRICS="${ENABLE_METRICS:-}"
TIMEOUT="${TIMEOUT:-10s}"

while [ "${1:-}" != "" ]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    -o|--outdir)
      OUTDIR="${2:-}"; shift 2
      ;;
    -e|--engines)
      ENGINES="${2:-}"; shift 2
      ;;
    -f|--filter)
      FILTER="${2:-}"; shift 2
      ;;
    -t|--timeout)
      TIMEOUT="${2:-}"; shift 2
      ;;
    --rebar)
      SEARCH_ROOT="${2:-}"; shift 2
      ;;
    --enable-metrics)
      ENABLE_METRICS=1; shift
      ;;
    --)
      shift
      break
      ;;
    *)
      echo "Unknown option: $1" >&2
      echo "Run with --help for usage." >&2
      exit 2
      ;;
  esac
done

if [ -z "$OUTDIR" ] || [ -z "$ENGINES" ]; then
  echo "Error: missing required option value. Run with --help." >&2
  exit 2
fi

# Ensure output directory exists and is empty. If it already exists, truncate it.
if [ -d "$OUTDIR" ]; then
  rm -rf "$OUTDIR"
fi
mkdir -p "$OUTDIR"

# Record start time for this run.
START_AT_UTC="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

# Determine the `rebar` repository root and run all commands from there.
if [ -z "$SEARCH_ROOT" ]; then
  # Default to a sibling "rebar" repo next to this "re3" repo.
  SEARCH_ROOT="${SELF_DIR}/../../rebar"
fi

if [ ! -d "$SEARCH_ROOT" ]; then
  echo "Error: rebar repository not found at: $SEARCH_ROOT" >&2
  echo "Set REBAR or pass --rebar to override." >&2
  exit 1
fi

ORIG_PWD="$(pwd)"
trap 'cd "$ORIG_PWD" >/dev/null 2>&1 || true' EXIT
cd "$SEARCH_ROOT"

# Locate the `rebar` executable under the repo's target directory.
if [ -x "target/release/rebar" ]; then
  REBAR_CMD="$PWD/target/release/rebar"
elif [ -x "target/debug/rebar" ]; then
  REBAR_CMD="$PWD/target/debug/rebar"
else
  echo "Error: rebar binary not found under: $SEARCH_ROOT/target" >&2
  echo "Build it with:" >&2
  echo "  cd \"$SEARCH_ROOT\" && cargo build --release" >&2
  exit 1
fi

# Write initial manifest describing this run configuration.
MANIFEST_PATH="${OUTDIR}/manifest.csv"
cat > "$MANIFEST_PATH" <<EOF
key,value
start_at_utc,$START_AT_UTC
script_path,$SELF_PATH
engines,$ENGINES
filter,${FILTER:-}
enable_metrics,${ENABLE_METRICS:-0}
timeout,${TIMEOUT:-10s}
EOF

# Build the re3 engine, the regexp engine
IFS_backup="$IFS"
IFS='|'
for engine in $ENGINES; do
  engine_trimmed="$(echo "$engine" | xargs)"
  echo "Building $engine_trimmed engine..."
  "$REBAR_CMD" build -e "$engine_trimmed" || exit 1
done
IFS="$IFS_backup"

# Run all benchmarks in one go; FILTER and ENGINES are passed through to rebar.
ALL_RESULTS="${OUTDIR}/benchmarks.csv"
REPORT_PATH="${OUTDIR}/report.md"
METRICS_PATH="${OUTDIR}/metrics.log"

if [ -n "$ENABLE_METRICS" ]; then
  export OTEL_ENABLED=true
  export OTEL_FILE_PATH="$METRICS_PATH"
  : > "$METRICS_PATH"
  echo "OpenTelemetry enabled; output: $METRICS_PATH"
fi

echo "Running benchmarks..."
"$REBAR_CMD" measure -e "$ENGINES" --timeout "$TIMEOUT" ${FILTER:+ -f "$FILTER"} | tee "$ALL_RESULTS"

# Record end time for this run.
END_AT_UTC="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
{
  echo "end_at_utc,$END_AT_UTC"
} >> "$MANIFEST_PATH"

echo "Generating report..."

"$REBAR_CMD" report "$ALL_RESULTS" | tee "$REPORT_PATH"

echo "Report generated at: $REPORT_PATH"

echo "Done."
