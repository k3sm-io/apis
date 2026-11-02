#!/usr/bin/env bash
# Sourced by hack/ci.sh: pick_staticcheck chooses the staticcheck binary the gate runs.
#
# Candidates, in order: $(go env GOPATH)/bin/staticcheck-<pin> (the versioned
# name, so two pins can be installed side by side while the repos switch), then
# staticcheck on PATH, then $(go env GOPATH)/bin/staticcheck. Every candidate is
# probed and the first whose version matches exactly wins, so a stale copy does
# not shadow a correct one. No match is RED, never a skip.
#
# Needs STATICCHECK_VERSION. Sets sc to the chosen binary and returns 0, or prints
# the named red to stderr and returns 1. Pinned by hack/lib/staticcheck_pick_test.go.
pick_staticcheck() {
	sc=""
	local sc_seen="" sc_cands="" sc_ver c
	local sc_gopath
	sc_gopath="$(go env GOPATH)"
	local sc_hint="go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}; or, beside another pin: GOBIN=<tmp> go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION} && mv <tmp>/staticcheck ${sc_gopath}/bin/staticcheck-${STATICCHECK_VERSION}"
	for c in "${sc_gopath}/bin/staticcheck-${STATICCHECK_VERSION}" "$(command -v staticcheck 2>/dev/null || true)" "${sc_gopath}/bin/staticcheck"; do
		[ -n "$c" ] && [ -x "$c" ] || continue
		case $'\n'"${sc_cands}"$'\n' in *$'\n'"${c}"$'\n'*) continue ;; esac
		sc_cands="${sc_cands:+${sc_cands}$'\n'}${c}"
	done
	while IFS= read -r c; do
		[ -n "$c" ] || continue
		sc_ver="$("$c" -version 2>/dev/null | awk '{print $2}' || true)"
		sc_seen="${sc_seen}  ${c}: ${sc_ver:-unknown}"$'\n'
		if [ -z "$sc" ] && [ "$sc_ver" = "$STATICCHECK_VERSION" ]; then sc="$c"; fi
	done <<< "$sc_cands"
	if [ -z "$sc" ]; then
		if [ -z "$sc_seen" ]; then
			echo "==> [apis] staticcheck: not installed; the gate needs ${STATICCHECK_VERSION} (${sc_hint})" >&2
		else
			echo "==> [apis] staticcheck: no candidate matches; the gate needs ${STATICCHECK_VERSION}:" >&2
			printf '%s' "$sc_seen" >&2
			echo "    (${sc_hint})" >&2
		fi
		return 1
	fi
}
