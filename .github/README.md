# GitHub Actions Workflows

This directory contains the GitHub Actions workflows for the Chaperone Auth Gateway project.

## Workflows Overview

### 1. CI Tests (`test.yml`)

**Trigger:**
- Push to `main` or `master` branches
- Pull requests targeting `main` or `master`

**What it does:**
- Runs the full test suite with race detection
- Performs linting with `golangci-lint`
- Checks code formatting
- Builds the binary

**Environment:** Ubuntu Latest

### 2. Beta Release (`release-beta.yml`)

**Trigger:**
- Push to `main` or `master` branches (after tests pass)

**What it does:**
- Runs all tests first
- Builds binaries for multiple platforms:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
- Creates version tags in format: `beta-YYYYMMDD-COMMIT`
- Creates/updates the `beta` tag to always point to the latest
- Publishes a pre-release on GitHub

**Artifacts produced:**
- `chaperone-beta-YYYYMMDD-COMMIT-PLATFORM-ARCH.tar.gz`
- Corresponding SHA256 checksum files

### 3. Production Release (`release-production.yml`)

**Trigger:**
- Manual workflow dispatch (from GitHub Actions tab)

**Options:**
- **Release type:** Patch, Minor, or Major
- **Dry run:** Simulate the release without actually publishing

**What it does:**
- Runs all tests first
- Calculates the next version number based on the latest release
- Builds binaries for multiple platforms:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
  - Windows (amd64)
- Creates a Git tag with the new version
- Generates release notes with commit history
- Publishes a full release on GitHub

**Artifacts produced:**
- `chaperone-vX.Y.Z-PLATFORM-ARCH.tar.gz` (or .zip for Windows)
- Corresponding SHA256 checksum files

## How to Use

### For Beta Releases (Automatic)

Simply push or merge to the `main`/`master` branch. The workflow will automatically:
1. Run tests
2. Build binaries
3. Update the `beta` tag
4. Create/update a pre-release

### For Production Releases (Manual)

1. Go to the **Actions** tab in your GitHub repository
2. Select **Release Production** workflow
3. Click **Run workflow**
4. Choose:
   - **Release type:** patch, minor, or major
   - **Dry run:** Check this to test without releasing
5. Click **Run workflow**

The workflow will:
- Calculate the next version (e.g., 1.2.3 → 1.2.4 for patch)
- Run tests and build binaries
- Create a Git tag like `v1.2.4`
- Publish a full release with automatic release notes

## Security Note

Make sure your repository has the following secrets configured (if needed):
- `GITHUB_TOKEN`: Automatically provided by GitHub Actions

## Local Development

To test the build process locally:

```bash
# Install dependencies
go mod download

# Run tests
make test-race

# Build for your current platform
make build

# Cross-build for other platforms
GOOS=linux GOARCH=amd64 go build -o chaperone-linux-amd64 ./cmd/chaperone
GOOS=windows GOARCH=amd64 go build -o chaperone-windows-amd64.exe ./cmd/chaperone
```