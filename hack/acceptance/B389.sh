#!/usr/bin/env bash
#
# apis B389 acceptance gate — hack/gen.sh must reinstall a codegen tool whose
# --version is not the pinned one, not merely one that is absent.
#
# THE ITEM (B389): gen.sh's old need() was presence-only (`command -v`), so a
# stale pre-bump buf / protoc-gen-go / protoc-gen-go-grpc left in GOPATH/bin
# silently won over a bumped pin in hack/lib/buf-env.sh, and the next
# regenerate used the old toolchain — breaking the reproducible-codegen
# contract gen.sh's own header states. The fix adds ONE detection helper to
# buf-env.sh (tool_version_matches: last field of `<cmd> --version`, leading
# `v` stripped from both sides, exact compare; any error or empty output is
# "no match") that gen.sh uses to decide a reinstall and ci.sh uses to WARN.
#
# Hermetic: GOPATH is re-pointed at an empty scratch dir before anything
# sources buf-env.sh (so its GOBIN prepend cannot shadow the fixtures with a
# real toolchain); `go` is PATH-shadowed by a wrapper that passes everything
# through to the real go EXCEPT `install`, which it appends to a sentinel log
# and exits 0 — no network, no real install, no module-cache dependence. The
# fake tools print each tool's REAL --version shape. Expected versions come
# from sourcing hack/lib/buf-env.sh at run time, never a copied pin. gen.sh is
# exercised as a scratch copy, so its `buf dep update` / `buf generate` hit
# the fake buf and never touch this tree. Assertions are on the sentinel log
# and exit codes only.
#
#   b389.1  BLOCKING  wiring: buf-env.sh defines tool_version_matches; gen.sh
#                     and ci.sh both call it; ci.sh carries no ad-hoc
#                     `"v$(buf --version` transform of its own.
#   b389.2  BLOCKING  helper table: pinned version in each tool's real shape
#                     matches; wrong / garbled / erroring / empty / absent do
#                     not; the fixtures (not a real toolchain) resolve.
#   b389.3  BLOCKING  gen.sh with all three tools at their pins exits 0 and
#                     installs nothing (sentinel empty).
#   b389.4  BLOCKING  gen.sh with all three at a wrong version reinstalls each
#                     at exactly its pin (<module>@<pin> per tool).
#   b389.5  BLOCKING  gen.sh with only buf wrong reinstalls only buf.
#   b389.6  BLOCKING  gen.sh with a garbled, an erroring (right-looking stdout,
#                     non-zero exit), and an empty --version reinstalls all
#                     three — the check fails toward reinstall.
#   b389.7  BLOCKING  sourcing gen.sh installs and generates nothing, and
#                     exposes needs_install (true for wrong, false for pinned).
#   b389.8  BLOCKING  gen.sh, buf-env.sh, ci.sh and this gate parse (`bash -n`).
#
# RED BEFORE THE WORK: with only gen.sh reverted to the presence-only need(),
# b389.4, b389.5 and b389.6 fail (the stale binaries are present, so nothing is
# reinstalled and the sentinel log stays empty), b389.1b fails (gen.sh does not
# call the shared helper), and b389.7 fails (no sourceable needs_install).
#
# Usage: bash hack/acceptance/B389.sh
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
GEN="$ROOT/hack/gen.sh"
BUFENV="$ROOT/hack/lib/buf-env.sh"
CI="$ROOT/hack/ci.sh"

echo "==> B389 acceptance (version-aware codegen toolchain install; repo: $ROOT)"

PASS=0
FAIL=0
ladder() { if [ "$1" = ok ]; then echo "PASS  $2"; PASS=$((PASS + 1)); else echo "FAIL  $2"; FAIL=$((FAIL + 1)); fi; }
die() {
	echo "FAIL  $1" >&2
	echo
	echo "==> B389: $PASS passed, $((FAIL + 1)) failed"
	exit 1
}

for f in "$GEN" "$BUFENV" "$CI"; do
	[ -f "$f" ] || die "b389.0 $f does not exist"
done
REAL_GO="$(command -v go)" || die "b389.0 no go on PATH (the wrapper passes through to it)"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/b389.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# ---- hermetic environment --------------------------------------------------
export GOPATH="$WORK/gopath"      # empty: buf-env's GOBIN prepend resolves nothing
export GOTOOLCHAIN=local          # never let `go env` fetch a toolchain
mkdir -p "$GOPATH/bin" "$WORK/gowrap"
export SENTINEL="$WORK/installs.log"
export BUFCALLS="$WORK/bufcalls.log"
: >"$SENTINEL"
: >"$BUFCALLS"
cat >"$WORK/gowrap/go" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = install ]; then
	shift
	printf '%s\n' "\$*" >>"\$SENTINEL"
	exit 0
fi
exec "$REAL_GO" "\$@"
EOF
chmod +x "$WORK/gowrap/go"
BASE_PATH="$WORK/gowrap:$PATH"

# Expected versions come from buf-env.sh itself, sourced under the scratch GOPATH.
cd "$WORK"
PATH="$BASE_PATH"
# shellcheck source=/dev/null
. "$BUFENV"
[ "$GOBIN" = "$GOPATH/bin" ] || die "b389.0 buf-env GOBIN is $GOBIN, not the scratch $GOPATH/bin"
for v in BUF_VERSION PROTOC_GEN_GO_VERSION PROTOC_GEN_GO_GRPC_VERSION; do
	[ -n "${!v:-}" ] || die "b389.0 buf-env.sh does not set $v"
done
PIN_BUF="$BUF_VERSION"
PIN_GO="$PROTOC_GEN_GO_VERSION"
PIN_GRPC="$PROTOC_GEN_GO_GRPC_VERSION"
MOD_BUF=github.com/bufbuild/buf/cmd/buf
MOD_GO=google.golang.org/protobuf/cmd/protoc-gen-go
MOD_GRPC=google.golang.org/grpc/cmd/protoc-gen-go-grpc

# mkfake <dir> <tool> <mode> <pin> — a fake tool printing its REAL --version
# shape. mode: pinned | wrong | garbled | error | empty. buf also accepts any
# other subcommand (dep update, generate) as a logged no-op.
mkfake() {
	local dir="$1" tool="$2" mode="$3" pin="${4#v}" ver line rc=0
	case "$mode" in
	pinned) ver="$pin" ;;
	wrong) ver="0.0.1" ;;
	*) ver="$pin" ;;
	esac
	case "$tool" in
	buf) line="$ver" ;;                                # 1.73.0
	protoc-gen-go) line="protoc-gen-go v$ver" ;;       # protoc-gen-go v1.36.12
	protoc-gen-go-grpc) line="protoc-gen-go-grpc $ver" ;; # protoc-gen-go-grpc 1.6.2
	esac
	case "$mode" in
	garbled) line="garbage: unknown flag" ;;
	error) rc=2 ;;                                     # right-looking stdout, failing exit
	empty) line="" ;;
	esac
	mkdir -p "$dir"
	cat >"$dir/$tool" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = --version ]; then
	[ -n "$line" ] && printf '%s\n' "$line"
	exit $rc
fi
printf '%s %s\n' "$tool" "\$*" >>"\$BUFCALLS"
exit 0
EOF
	chmod +x "$dir/$tool"
}

# scenario <name> <buf-mode> <go-mode> <grpc-mode> — builds a fixture dir and
# a scratch copy of gen.sh + buf-env.sh; echoes the fixture dir.
scenario() {
	local d="$WORK/$1"
	mkfake "$d/bin" buf "$2" "$PIN_BUF"
	mkfake "$d/bin" protoc-gen-go "$3" "$PIN_GO"
	mkfake "$d/bin" protoc-gen-go-grpc "$4" "$PIN_GRPC"
	mkdir -p "$d/repo/hack/lib"
	cp "$GEN" "$d/repo/hack/gen.sh"
	cp "$BUFENV" "$d/repo/hack/lib/buf-env.sh"
	echo "$d"
}

# rungen <scenario-dir> — runs the scratch gen.sh with the fixtures on PATH;
# sets RC and INSTALLS (the sentinel lines this run appended).
rungen() {
	: >"$SENTINEL"
	: >"$BUFCALLS"
	RC=0
	(cd "$WORK" && PATH="$1/bin:$BASE_PATH" bash "$1/repo/hack/gen.sh") >"$1/out.log" 2>&1 || RC=$?
	INSTALLS="$(sort "$SENTINEL")"
}

# ==== b389.1 — one detection, two reactions ================================
if grep -qE '^[[:space:]]*tool_version_matches\(\)' "$BUFENV"; then
	ladder ok "b389.1a buf-env.sh defines tool_version_matches"
else
	ladder no "b389.1a buf-env.sh does not define tool_version_matches"
fi
if grep -q 'tool_version_matches' "$GEN"; then
	ladder ok "b389.1b gen.sh calls the shared tool_version_matches"
else
	ladder no "b389.1b gen.sh does not call tool_version_matches"
fi
if grep -q 'tool_version_matches buf' "$CI" && ! grep -qF '"v$(buf --version' "$CI"; then
	ladder ok "b389.1c ci.sh calls tool_version_matches for buf and has no ad-hoc transform"
else
	ladder no "b389.1c ci.sh does not use the shared helper (or keeps its own buf --version transform)"
fi

# ==== b389.2 — the helper table =============================================
if declare -F tool_version_matches >/dev/null; then
	H="$WORK/helper/bin"
	mkfake "$H/pinned" buf pinned "$PIN_BUF"
	mkfake "$H/pinned" protoc-gen-go pinned "$PIN_GO"
	mkfake "$H/pinned" protoc-gen-go-grpc pinned "$PIN_GRPC"
	h_ok=1
	for t in "buf $PIN_BUF" "protoc-gen-go $PIN_GO" "protoc-gen-go-grpc $PIN_GRPC"; do
		set -- $t
		tool="$1" pin="$2"
		PATH="$H/pinned:$BASE_PATH"
		if [ "$(command -v "$tool")" != "$H/pinned/$tool" ]; then
			ladder no "b389.2 $tool does not resolve to the fixture (resolved: $(command -v "$tool" || echo none))"
			h_ok=0
		fi
		tool_version_matches "$tool" "$pin" || { ladder no "b389.2 pinned $tool ($pin) in its real shape did not match"; h_ok=0; }
		for m in wrong garbled error empty; do
			mkfake "$H/$m" "$tool" "$m" "$pin"
			PATH="$H/$m:$BASE_PATH"
			if tool_version_matches "$tool" "$pin"; then
				ladder no "b389.2 $m $tool matched the pin $pin (must fail toward reinstall)"
				h_ok=0
			fi
		done
		PATH="$BASE_PATH"
		if tool_version_matches "$tool" "$pin"; then
			ladder no "b389.2 absent $tool matched (a real toolchain leaked onto PATH?)"
			h_ok=0
		fi
	done
	PATH="$BASE_PATH"
	[ "$h_ok" = 1 ] && ladder ok "b389.2 helper: pinned matches in all three real shapes; wrong/garbled/error/empty/absent do not"
else
	ladder no "b389.2 tool_version_matches is not defined after sourcing buf-env.sh"
fi

EXP_ALL="$(printf '%s\n' "$MOD_BUF@$PIN_BUF" "$MOD_GO@$PIN_GO" "$MOD_GRPC@$PIN_GRPC" | sort)"

# ==== b389.3 — all pinned: untouched ========================================
d="$(scenario s3 pinned pinned pinned)"
rungen "$d"
if [ "$RC" = 0 ] && [ -z "$INSTALLS" ]; then
	ladder ok "b389.3 all tools at their pins: gen.sh exit 0, nothing reinstalled"
else
	ladder no "b389.3 all pinned: exit $RC, installs: [${INSTALLS//$'\n'/, }]"
	sed 's/^/      /' "$d/out.log"
fi

# ==== b389.4 — all wrong: each reinstalled at its pin =======================
d="$(scenario s4 wrong wrong wrong)"
rungen "$d"
if [ "$RC" = 0 ] && [ "$INSTALLS" = "$EXP_ALL" ]; then
	ladder ok "b389.4 all tools at a wrong version: each reinstalled at exactly its pin"
else
	ladder no "b389.4 all wrong: exit $RC, installs: [${INSTALLS//$'\n'/, }], want [${EXP_ALL//$'\n'/, }]"
fi

# ==== b389.5 — only buf wrong: only buf reinstalled =========================
d="$(scenario s5 wrong pinned pinned)"
rungen "$d"
if [ "$RC" = 0 ] && [ "$INSTALLS" = "$MOD_BUF@$PIN_BUF" ]; then
	ladder ok "b389.5 only buf wrong: only buf reinstalled"
else
	ladder no "b389.5 buf wrong: exit $RC, installs: [${INSTALLS//$'\n'/, }], want [$MOD_BUF@$PIN_BUF]"
fi

# ==== b389.6 — garbled / erroring / empty: fail toward reinstall ============
d="$(scenario s6 garbled error empty)"
rungen "$d"
if [ "$RC" = 0 ] && [ "$INSTALLS" = "$EXP_ALL" ]; then
	ladder ok "b389.6 garbled (buf), erroring (protoc-gen-go), empty (grpc) --version: all reinstalled"
else
	ladder no "b389.6 garbled/error/empty: exit $RC, installs: [${INSTALLS//$'\n'/, }], want [${EXP_ALL//$'\n'/, }]"
fi

# ==== b389.7 — sourcing gen.sh runs nothing; needs_install is drivable ======
d="$(scenario s7 wrong pinned pinned)"
: >"$SENTINEL"
: >"$BUFCALLS"
s7="$( (
	cd "$WORK"
	PATH="$d/bin:$BASE_PATH"
	# shellcheck source=/dev/null
	. "$d/repo/hack/gen.sh" >/dev/null 2>&1 </dev/null || true
	declare -F needs_install >/dev/null || { echo "no-needs_install"; exit 0; }
	needs_install buf "$PIN_BUF" && echo "buf-wrong:install" || echo "buf-wrong:keep"
	needs_install protoc-gen-go "$PIN_GO" && echo "go-pinned:install" || echo "go-pinned:keep"
) 2>&1 || true)"
if [ ! -s "$SENTINEL" ] && [ ! -s "$BUFCALLS" ] && [ "$s7" = "$(printf 'buf-wrong:install\ngo-pinned:keep')" ]; then
	ladder ok "b389.7 sourcing gen.sh ran nothing; needs_install: wrong→install, pinned→keep"
else
	ladder no "b389.7 source gen.sh: installs=[$(tr '\n' ' ' <"$SENTINEL")] buf-calls=[$(tr '\n' ' ' <"$BUFCALLS")] needs_install=[${s7//$'\n'/, }]"
fi

# ==== b389.8 — everything parses ============================================
p_ok=1
for f in "$GEN" "$BUFENV" "$CI" "$HERE/B389.sh"; do
	bash -n "$f" || { ladder no "b389.8 $f does not parse"; p_ok=0; }
done
[ "$p_ok" = 1 ] && ladder ok "b389.8 gen.sh, buf-env.sh, ci.sh and this gate parse"

echo
echo "==> B389: $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ]
