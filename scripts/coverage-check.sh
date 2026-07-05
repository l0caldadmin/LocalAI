#!/usr/bin/env sh
#
# coverage-check.sh PROFILE BASELINE_FILE
#
# Compares the total statement coverage in a Go cover profile against a
# committed baseline and fails (exit 1) if it dropped by more than
# COVERAGE_TOLERANCE percentage points (default 0.5).
#
# A small tolerance is needed because the measured profile folds in the
# in-process tests/e2e suite (via --coverpkg): which handler lines that suite
# executes depends on request timing/flake-retries, so the total jitters a
# little run-to-run even with no code change. Keep the tolerance just above the
# observed jitter; the unit/suite portion is deterministic.
#
# When coverage rises meaningfully, regenerate and commit the baseline with:
#   make test-coverage-baseline
set -eu

profile="${1:?usage: coverage-check.sh PROFILE BASELINE_FILE}"
baseline_file="${2:?usage: coverage-check.sh PROFILE BASELINE_FILE}"
tolerance="${COVERAGE_TOLERANCE:-0.5}"

if [ ! -f "$profile" ]; then
	echo "coverage-check: profile not found: $profile" >&2
	exit 2
fi
if [ ! -f "$baseline_file" ]; then
	echo "coverage-check: baseline not found: $baseline_file" >&2
	echo "coverage-check: create it with 'make test-coverage-baseline'" >&2
	exit 2
fi

current="$(go tool cover -func="$profile" 2>/dev/null | awk '/^total:/{gsub(/%/,"",$NF); print $NF}')" || true

if [ -z "$current" ]; then
	# go tool cover returns "[no statements]" and exits non-zero when the
	# merged profile has no data for the selected --coverpkg scope. This
	# happens on the first CI run after a coverpkg scope change (before the
	# new baseline is captured with 'make test-coverage-baseline').
	# Treat as 0% — the baseline is also 0 in that case, so the gate passes.
	echo "coverage-check: profile has no measurable statements for the selected coverpkg."
	echo "coverage-check: treating as 0%. Run 'make test-coverage-baseline' after a green run."
	current="0"
fi
baseline="$(tr -d '[:space:]%' < "$baseline_file")"

# Fail closed on a missing/garbage baseline rather than letting awk coerce an
# empty or non-numeric value to 0 (which would pass any coverage silently).
case "$baseline" in
	'' | *[!0-9.]* )
		echo "coverage-check: baseline is empty or non-numeric ('$baseline') in $baseline_file" >&2
		echo "coverage-check: regenerate it with 'make test-coverage-baseline'" >&2
		exit 2 ;;
esac

# Compare as floats. Fail only when current is below baseline by more than the
# tolerance.
if awk -v c="$current" -v b="$baseline" -v t="$tolerance" 'BEGIN { exit !(c < b - t) }'; then
	echo "coverage-check: FAIL — coverage ${current}% is below baseline ${baseline}% by more than ${tolerance}pp." >&2
	echo "coverage-check: coverage regressed beyond the jitter tolerance. Add tests to restore it." >&2
	exit 1
fi

if awk -v c="$current" -v b="$baseline" 'BEGIN { exit !(c > b) }'; then
	echo "coverage-check: OK — coverage rose to ${current}% (baseline ${baseline}%); consider 'make test-coverage-baseline'."
else
	echo "coverage-check: OK — coverage ${current}% within ${tolerance}pp of baseline ${baseline}%."
fi
