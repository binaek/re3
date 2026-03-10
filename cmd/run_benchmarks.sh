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
      Only run benchmarks whose "name,model" matches REGEX (grep -E syntax).
      Example: --filter '^test/model'
      Env: BENCH_FILTER

  --rebar DIR
      Path to the `rebar` repository root (its working directory).
      Default: sibling "rebar" repo next to this "re3" repo
      Env: REBAR

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
BENCH_FILTER="${BENCH_FILTER:-}"
ENGINES="${ENGINES:-go/regexp|go/re3}"
SEARCH_ROOT="${REBAR:-}"

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
      BENCH_FILTER="${2:-}"; shift 2
      ;;
    --rebar)
      SEARCH_ROOT="${2:-}"; shift 2
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
MANIFEST_PATH="${OUTDIR}/0a_manifest.csv"
cat > "$MANIFEST_PATH" <<EOF
key,value
start_at_utc,$START_AT_UTC
script_path,$SELF_PATH
engines,$ENGINES
bench_filter,${BENCH_FILTER:-}
EOF

# Get unique (name,model) pairs that have a go/re3 implementation.
LIST="$("$REBAR_CMD" measure --list | grep 'go/re3' | cut -d',' -f1-2 | sort -u)"
if [ -n "$BENCH_FILTER" ]; then
  LIST="$(printf '%s\n' "$LIST" | grep -E "$BENCH_FILTER" || true)"
fi

if [ -z "$LIST" ]; then
  echo "No benchmarks matched filter '\$BENCH_FILTER'. Nothing to do." >&2
  exit 0
fi

# Build the re3 engine, the regexp engine, and the rust/regex engine
echo "Building re3 engine..."
"$REBAR_CMD" build -e go/re3 || exit 1

echo "Building regexp engine..."
"$REBAR_CMD" build -e go/regexp || exit 1

# Prepare manifests; rows are appended after each benchmark runs.
BENCHMARKS_MANIFEST="${OUTDIR}/0b_benchmarks.csv"
echo "name,model,output_file,start_utc,end_utc" > "$BENCHMARKS_MANIFEST"
ALL_RESULTS="${OUTDIR}/0c_all_results.csv"
ALL_HEADER_WRITTEN=0

# Generate a report summarizing the run.
REPORT_PATH="${OUTDIR}/0d_report.md"

printf '%s\n' "$LIST" | while IFS=, read -r name model; do
  id="${name},${model}"
  # Make a filesystem‑safe filename from name+model.
  safe_name="${name//\//__}__${model}"
  outfile="${OUTDIR}/${safe_name}.csv"

  echo "--------------------------------"
  echo "Running ${id} -> ${outfile}"

  # Record start time for this run.
  bench_start_at_utc="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

  # Run only benchmarks for this name, then filter by exact (name,model)
  # to isolate a single benchmark definition in the output.
  "$REBAR_CMD" measure -e "$ENGINES" -f "^${name}" \
    | awk -F',' -v n="$name" -v m="$model" 'NR==1 || ($1 == n && $2 == m)' \
    | tee "$outfile"

  bench_end_at_utc="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"

  # Only once the benchmark has completed do we record it in the manifests.
  echo "$name,$model,$outfile,$bench_start_at_utc,$bench_end_at_utc" >> "$BENCHMARKS_MANIFEST"

  # Append into the "all results" CSV incrementally.
  if [ "$ALL_HEADER_WRITTEN" -eq 0 ]; then
    # First benchmark: copy its file as-is, including header.
    cat "$outfile" > "$ALL_RESULTS"
    ALL_HEADER_WRITTEN=1
  else
    # Subsequent benchmarks: append without header.
    tail -n +2 "$outfile" >> "$ALL_RESULTS"
  fi
done

# Record end time for this run.
END_AT_UTC="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
{
  echo "end_at_utc,$END_AT_UTC"
} >> "$MANIFEST_PATH"

echo "Generating report..."
"$REBAR_CMD" report "$ALL_RESULTS" > "$REPORT_PATH"

echo "Report generated at: $REPORT_PATH"

echo "Done."