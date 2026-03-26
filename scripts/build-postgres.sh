#!/usr/bin/env bash
# build-postgres.sh — Build AYB-managed Postgres binaries
#
# Usage:
#   ./scripts/build-postgres.sh [OPTIONS]
#
# Options:
#   --pg-version  PG major version to build (default: 16)
#   --os          Target OS: linux or darwin (default: current OS)
#   --arch        Target arch: amd64 or arm64 (default: current arch)
#   --output-dir  Directory for output tarballs (default: ./dist/pg-binaries)
#
# The script produces:
#   ayb-postgres-{version}-{os}-{arch}.tar.xz
#   SHA256SUMS
#
# Requirements (installed by the script if missing on CI):
#   gcc, make, curl, xz, openssl-dev, libxml2-dev, uuid-dev (linux)
#   Xcode command line tools (darwin)

set -euo pipefail

# ---- Defaults ----
PG_VERSION="${PG_VERSION:-16}"
TARGET_OS="${TARGET_OS:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
TARGET_ARCH="${TARGET_ARCH:-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')}"
OUTPUT_DIR="${OUTPUT_DIR:-dist/pg-binaries}"

# ---- Extension versions ----
PGVECTOR_VERSION="${PGVECTOR_VERSION:-0.8.0}"
PG_CRON_VERSION="${PG_CRON_VERSION:-1.6.4}"

# ---- Argument parsing ----
while [[ $# -gt 0 ]]; do
  case $1 in
    --pg-version)  PG_VERSION="$2";   shift 2 ;;
    --os)          TARGET_OS="$2";    shift 2 ;;
    --arch)        TARGET_ARCH="$2";  shift 2 ;;
    --output-dir)  OUTPUT_DIR="$2";   shift 2 ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done

PLATFORM="${TARGET_OS}-${TARGET_ARCH}"
ARCHIVE_NAME="ayb-postgres-${PG_VERSION}-${PLATFORM}.tar.xz"
INSTALL_DIR="${OUTPUT_DIR}/ayb-postgres-${PG_VERSION}"
BUILD_DIR="${OUTPUT_DIR}/build"

echo "Building ayb-postgres ${PG_VERSION} for ${PLATFORM}"
echo "Output: ${OUTPUT_DIR}/${ARCHIVE_NAME}"

mkdir -p "${BUILD_DIR}" "${OUTPUT_DIR}"

# ---- Fetch Postgres source ----
PG_FULL_VERSION="${PG_VERSION}.$(curl -s "https://ftp.postgresql.org/pub/source/" \
  | grep -oE "v${PG_VERSION}\.[0-9]+" | sort -V | tail -1 | tr -d 'v' | cut -d. -f2)"
PG_FULL_VERSION="${PG_FULL_VERSION:-${PG_VERSION}.0}"

PG_SRC_URL="https://ftp.postgresql.org/pub/source/v${PG_FULL_VERSION}/postgresql-${PG_FULL_VERSION}.tar.bz2"
PG_SRC="${BUILD_DIR}/postgresql-${PG_FULL_VERSION}.tar.bz2"

if [ ! -f "${PG_SRC}" ]; then
  echo "Downloading PostgreSQL ${PG_FULL_VERSION}..."
  curl -fsSL -o "${PG_SRC}" "${PG_SRC_URL}"
fi

# ---- Build Postgres ----
PG_BUILD="${BUILD_DIR}/postgresql-${PG_FULL_VERSION}"
if [ ! -d "${PG_BUILD}" ]; then
  echo "Extracting PostgreSQL source..."
  tar -xjf "${PG_SRC}" -C "${BUILD_DIR}"
fi

if [ ! -f "${PG_BUILD}/src/backend/postgres" ]; then
  echo "Configuring and building PostgreSQL..."
  cd "${PG_BUILD}"
  ./configure \
    --prefix="${INSTALL_DIR}" \
    --with-openssl \
    --with-libxml \
    --with-uuid=e2fs \
    --without-readline \
    --without-zlib \
    --without-icu \
    CFLAGS="-O2"
  make -j"$(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo 4)"
  make install
  cd - > /dev/null
fi

# ---- Build pgvector ----
PGVECTOR_SRC="${BUILD_DIR}/pgvector-${PGVECTOR_VERSION}"
PGVECTOR_URL="https://github.com/pgvector/pgvector/archive/refs/tags/v${PGVECTOR_VERSION}.tar.gz"

if [ ! -d "${PGVECTOR_SRC}" ]; then
  echo "Downloading pgvector ${PGVECTOR_VERSION}..."
  curl -fsSL "${PGVECTOR_URL}" | tar -xzf - -C "${BUILD_DIR}"
  mv "${BUILD_DIR}/pgvector-${PGVECTOR_VERSION}" "${PGVECTOR_SRC}" 2>/dev/null || true
fi

if [ ! -f "${INSTALL_DIR}/lib/vector.so" ]; then
  echo "Building pgvector..."
  cd "${PGVECTOR_SRC}"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config" install
  cd - > /dev/null
fi

# ---- Build pg_cron ----
PGCRON_SRC="${BUILD_DIR}/pg_cron-${PG_CRON_VERSION}"
PGCRON_URL="https://github.com/citusdata/pg_cron/archive/refs/tags/v${PG_CRON_VERSION}.tar.gz"

if [ ! -d "${PGCRON_SRC}" ]; then
  echo "Downloading pg_cron ${PG_CRON_VERSION}..."
  curl -fsSL "${PGCRON_URL}" | tar -xzf - -C "${BUILD_DIR}"
  mv "${BUILD_DIR}/pg_cron-${PG_CRON_VERSION}" "${PGCRON_SRC}" 2>/dev/null || true
fi

if [ ! -f "${INSTALL_DIR}/lib/pg_cron.so" ]; then
  echo "Building pg_cron..."
  cd "${PGCRON_SRC}"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config" install
  cd - > /dev/null
fi

# ---- Build pg_trgm (included in PG, just verify ----
if [ ! -f "${INSTALL_DIR}/lib/pg_trgm.so" ]; then
  echo "Building pg_trgm (contrib)..."
  cd "${PG_BUILD}/contrib/pg_trgm"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config" install
  cd - > /dev/null
fi

# ---- Build pg_stat_statements (shared_preload_libraries default) ----
if [ ! -f "${INSTALL_DIR}/lib/pg_stat_statements.so" ]; then
  echo "Building pg_stat_statements (contrib)..."
  cd "${PG_BUILD}/contrib/pg_stat_statements"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config"
  make PG_CONFIG="${INSTALL_DIR}/bin/pg_config" install
  cd - > /dev/null
fi

# ---- Strip binaries (reduce size) ----
echo "Stripping binaries..."
find "${INSTALL_DIR}/bin" -type f -exec strip {} \; 2>/dev/null || true
find "${INSTALL_DIR}/lib" -name "*.so" -exec strip {} \; 2>/dev/null || true

# ---- Package ----
echo "Creating ${ARCHIVE_NAME}..."
cd "${OUTPUT_DIR}"
tar -cJf "${ARCHIVE_NAME}" "ayb-postgres-${PG_VERSION}"
echo "Archive created: ${OUTPUT_DIR}/${ARCHIVE_NAME}"

# ---- Generate SHA256SUMS ----
cd "${OUTPUT_DIR}"
sha256sum "${ARCHIVE_NAME}" > SHA256SUMS || shasum -a 256 "${ARCHIVE_NAME}" > SHA256SUMS
echo "SHA256SUMS written to ${OUTPUT_DIR}/SHA256SUMS"

# ---- Summary ----
echo ""
echo "Build complete:"
echo "  Archive: ${OUTPUT_DIR}/${ARCHIVE_NAME}"
echo "  SHA256:  $(cat "${OUTPUT_DIR}/SHA256SUMS")"
