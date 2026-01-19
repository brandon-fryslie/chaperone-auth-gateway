#!/usr/bin/env bash
set -euo pipefail

# Chaperone Release Build Script
# Builds cross-platform binaries with version information embedded

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if VERSION file exists
if [[ ! -f VERSION ]]; then
    echo -e "${RED}ERROR: VERSION file not found${NC}"
    exit 1
fi

VERSION=$(cat VERSION | tr -d '\n')
echo -e "${GREEN}Building Chaperone v${VERSION}${NC}"

# Create dist directory
DIST_DIR="dist"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# Build targets (GOOS/GOARCH)
declare -a TARGETS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
)

# Build function
build_target() {
    local os=$1
    local arch=$2
    local output_name="chaperone-${os}-${arch}"

    echo -e "${YELLOW}Building ${output_name}...${NC}"

    GOOS="${os}" GOARCH="${arch}" go build \
        -ldflags "-X github.com/bmf/chaperone/cmd/chaperone/cmd.Version=${VERSION}" \
        -o "${DIST_DIR}/${output_name}" \
        ./cmd/chaperone

    # Make executable
    chmod +x "${DIST_DIR}/${output_name}"

    # Calculate SHA256
    if command -v sha256sum &> /dev/null; then
        sha256sum "${DIST_DIR}/${output_name}" > "${DIST_DIR}/${output_name}.sha256"
    elif command -v shasum &> /dev/null; then
        shasum -a 256 "${DIST_DIR}/${output_name}" > "${DIST_DIR}/${output_name}.sha256"
    fi

    echo -e "${GREEN}✓ Built ${output_name}${NC}"
}

# Build all targets
for target in "${TARGETS[@]}"; do
    IFS='/' read -r os arch <<< "$target"
    build_target "$os" "$arch"
done

# Create checksums file
echo -e "${YELLOW}Creating checksums file...${NC}"
cd "${DIST_DIR}"
if command -v sha256sum &> /dev/null; then
    sha256sum chaperone-* > checksums.txt
elif command -v shasum &> /dev/null; then
    shasum -a 256 chaperone-* > checksums.txt
fi
cd ..

# List built files
echo -e "${GREEN}Built files:${NC}"
ls -lh "${DIST_DIR}/"

echo -e "${GREEN}Release build complete!${NC}"
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Test binaries: ./dist/chaperone-<platform>-<arch> version"
echo "2. Create git tag: git tag -a v${VERSION} -m 'Release v${VERSION}'"
echo "3. Push tag: git push origin v${VERSION}"
echo "4. Create GitHub release with binaries from dist/"
