#!/usr/bin/env bash
# build-libghostty.sh — Cross-compile libghostty-vt-static for all supported platforms
# and vendor the artifacts under lib/ghostty/<platform>/.
#
# Requirements:
#   - zig (v0.13+)
#   - git
#
# Limitations:
#   - macOS targets (darwin-x86_64, darwin-aarch64) require a macOS host with
#     Xcode installed. Ghostty's build system explicitly does not support macOS
#     cross-compilation from Linux because it relies on `xcrun` to locate the
#     Apple SDK. Run this script on macOS to build those targets.
#
# Usage:
#   bash scripts/build-libghostty.sh              # build all targets (skips macOS on Linux)
#   bash scripts/build-libghostty.sh linux         # build only Linux targets
#   bash scripts/build-libghostty.sh darwin        # build only macOS targets (requires macOS host)

set -euo pipefail

GHOSTTY_REPO="https://github.com/ghostty-org/ghostty.git"
GHOSTTY_COMMIT="9e080c5a403475dcbee93c40eeb22cf6f92121f4"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIB_ROOT="${REPO_ROOT}/lib/ghostty"

FILTER="${1:-}"

# Detect host OS to decide which targets to attempt
HOST_OS="$(uname -s)"

# Portable .pc file templates — use ${pcfiledir} so paths work regardless of
# where the repo is checked out.
pc_content_linux() {
  cat <<'EOF'
prefix=${pcfiledir}/../..
includedir=${prefix}/include
libdir=${prefix}/lib

Name: libghostty-vt-static
URL: https://github.com/ghostty-org/ghostty
Description: Ghostty VT terminal emulation library (static)
Version: 0.1.0
Cflags: -I${includedir}
Libs: ${libdir}/libghostty-vt.a
Libs.private:
Requires.private:
EOF
}

pc_content_windows() {
  cat <<'EOF'
prefix=${pcfiledir}/../..
includedir=${prefix}/include
libdir=${prefix}/lib

Name: libghostty-vt-static
URL: https://github.com/ghostty-org/ghostty
Description: Ghostty VT terminal emulation library (static)
Version: 0.1.0
Cflags: -I${includedir}
Libs: ${libdir}/ghostty-vt-static.lib
Libs.private:
Requires.private:
EOF
}

# "zig-triple  output-platform-dir  pc-type  requires-macos"
declare -a TARGETS=(
  "x86_64-linux-gnu    linux-x86_64    linux  no"
  "aarch64-linux-gnu   linux-aarch64   linux  no"
  "x86_64-windows-gnu  windows-x86_64  win    no"
  "aarch64-windows-gnu windows-aarch64 win    no"
  "x86_64-macos        darwin-x86_64   linux  yes"
  "aarch64-macos       darwin-aarch64  linux  yes"
)

TMPDIR_BUILD="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_BUILD"' EXIT

echo "==> Cloning Ghostty at ${GHOSTTY_COMMIT} ..."
git clone --quiet --no-checkout "${GHOSTTY_REPO}" "${TMPDIR_BUILD}/ghostty"
cd "${TMPDIR_BUILD}/ghostty"
git checkout --quiet "${GHOSTTY_COMMIT}"

for entry in "${TARGETS[@]}"; do
  ZIG_TARGET="$(echo "$entry" | awk '{print $1}')"
  PLATFORM="$(echo "$entry" | awk '{print $2}')"
  PC_TYPE="$(echo "$entry" | awk '{print $3}')"
  NEEDS_MACOS="$(echo "$entry" | awk '{print $4}')"

  # Apply filter if given
  if [ -n "$FILTER" ] && [[ "$PLATFORM" != *"$FILTER"* ]]; then
    continue
  fi

  # Skip macOS targets on non-macOS hosts
  if [ "$NEEDS_MACOS" = "yes" ] && [ "$HOST_OS" != "Darwin" ]; then
    echo ""
    echo "==> Skipping ${PLATFORM} (${ZIG_TARGET}) — requires macOS host with Xcode"
    continue
  fi

  OUT_DIR="${LIB_ROOT}/${PLATFORM}"
  BUILD_OUT="${TMPDIR_BUILD}/out-${PLATFORM}"

  echo ""
  echo "==> Building for ${PLATFORM} (${ZIG_TARGET}) ..."

  mkdir -p "${BUILD_OUT}"

  zig build \
    -Doptimize=ReleaseFast \
    -Dtarget="${ZIG_TARGET}" \
    -Demit-lib-vt=true \
    -Dlib-version-string=0.1.0 \
    --prefix "${BUILD_OUT}" \
    install

  # Copy include/, lib/, and share/ into the vendor directory
  mkdir -p "${OUT_DIR}"
  for subdir in include lib share; do
    if [ -d "${BUILD_OUT}/${subdir}" ]; then
      rm -rf "${OUT_DIR:?}/${subdir}"
      cp -r "${BUILD_OUT}/${subdir}" "${OUT_DIR}/"
    fi
  done

  # Remove dynamic library files — we only need the static archive
  find "${OUT_DIR}/lib" \( -name "*.so*" -o -name "*.dylib" -o -name "ghostty-vt.lib" \) \
    -delete 2>/dev/null || true

  # Write portable pkg-config file
  PC_FILE="${OUT_DIR}/share/pkgconfig/libghostty-vt-static.pc"
  if [ -f "${PC_FILE}" ]; then
    if [ "$PC_TYPE" = "win" ]; then
      pc_content_windows > "${PC_FILE}"
    else
      pc_content_linux > "${PC_FILE}"
    fi
    echo "    Fixed ${PC_FILE}"
  fi

  echo "    Done -> ${OUT_DIR}"
done

echo ""
echo "==> Build complete. Artifacts are in ${LIB_ROOT}/"
if [ "$HOST_OS" != "Darwin" ]; then
  echo "    NOTE: macOS targets were skipped. Run on macOS with Xcode to build darwin-* libs."
fi
