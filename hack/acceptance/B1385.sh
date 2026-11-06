#!/usr/bin/env bash
# Copyright The k3sm Authors.
# SPDX-License-Identifier: Apache-2.0
#
# apis acceptance gate: the artifact-store wire names, landed contract first.
#
# --check runs, and requires each named test to PASS by name (a test that does
# not run is not a pass):
#   runtime/v1     TestImageVolumeSourceWire  Volume.image = 9, ImageVolumeSource
#                                             reference = 1, pull_policy = 2,
#                                             paths = 3, the union comments
#                  TestPullArtifactWire       Images.PullArtifact request and
#                                             response fields, the closed
#                                             ArtifactPolicy enum, the
#                                             idempotent and resumable text
#                  TestImagesServiceSurface   the Images surface, now eleven verbs
#   mlx/v1alpha1   TestMLXModelArtifactJSON   spec.artifact, status.resolvedArtifact,
#                                             no model key when artifact-only
#                  TestMLXModelTypesMatchCRDSchema  the Go types and the shipped
#                                             CRD schema agree both ways
#   config/crd     TestMLXModelArtifactFieldCEL  model out of spec.required, the
#                                             exactly-one-of CEL rule on spec
#                  TestMLXModelCRDValidatesDistributed  (updated: model optional)
#   artifact/v1    the media types, the per-consumer allow-sets, ValidatePath,
#                  ValidatePathSet, the annotation bounds, and FuzzValidatePath
#                  over its committed seeds
# and then `buf breaking` against buf/baseline.binpb when the pinned buf is
# installed (hack/gen.sh installs it); without it that step is reported SKIP.
#
# RED BEFORE THE WORK: none of the named tests exist, artifact/v1 and its
# seed corpus do not exist, so the gate fails at the first missing file.
#
# Usage: hack/acceptance/B1385.sh --check
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"

[ "${1:-}" = "--check" ] && [ $# -eq 1 ] || { echo "usage: $0 --check" >&2; exit 2; }

cd "$ROOT"
export CGO_ENABLED=0

for f in artifact/v1/path.go artifact/v1/allowset.go artifact/v1/annotation.go \
	mlx/v1alpha1/artifactref.go runtime/v1/artifact_wire_test.go \
	config/crd/mlxmodel_artifact_test.go; do
	[ -e "$f" ] || { echo "B1385: FAIL: $f is missing"; exit 1; }
done
seeds="artifact/v1/testdata/fuzz/FuzzValidatePath"
[ -d "$seeds" ] || { echo "B1385: FAIL: $seeds is missing"; exit 1; }
n="$(find "$seeds" -type f | wc -l | tr -d ' ')"
[ "$n" -ge 10 ] || { echo "B1385: FAIL: only $n committed FuzzValidatePath seeds, want at least 10"; exit 1; }

# run <package> <test>... runs exactly the named tests and requires each to pass.
run() {
	local pkg="$1"
	shift
	local re out t
	re="^($(IFS='|'; echo "$*"))\$"
	out="$(go test -count=1 -v -run "$re" "$pkg" 2>&1)" || {
		printf '%s\n' "$out"; echo "B1385: FAIL: $pkg"; exit 1; }
	printf '%s\n' "$out" | grep -E '^(--- |ok)'
	for t in "$@"; do
		grep -q -- "--- PASS: $t " <<<"$out" || { echo "B1385: FAIL: $t did not run"; exit 1; }
	done
}

run ./runtime/v1/ TestImageVolumeSourceWire TestPullArtifactWire TestImagesServiceSurface
run ./mlx/v1alpha1/ TestMLXModelArtifactJSON TestMLXModelTypesMatchCRDSchema
run ./config/crd/ TestMLXModelArtifactFieldCEL TestMLXModelCRDValidatesDistributed
run ./artifact/v1/ TestMediaTypeConstants TestArtifactType TestImageVolumeAllowSet \
	TestGuestAllowSetIsSeparate TestNoAllowSetAdmitsTarCodeOrImageConfig \
	TestAllowSetIsNotMutableThroughItsAccessors TestArtifactPathsAnnotationBounds \
	TestValidatePath TestValidatePathSet FuzzValidatePath

# shellcheck source=/dev/null
. "$ROOT/hack/lib/buf-env.sh"
if tool_version_matches buf "$BUF_VERSION"; then
	buf breaking --against buf/baseline.binpb || { echo "B1385: FAIL: buf breaking"; exit 1; }
	echo "ok  buf breaking"
else
	echo "SKIP buf breaking (pinned buf $BUF_VERSION not installed; run hack/gen.sh)"
fi
echo "B1385: PASS"
