# Release Setup

This project uses [GoReleaser](https://goreleaser.com) to build and publish binaries for multiple platforms, and publishes a Homebrew formula for easy installation.

## Initial Setup

### 1. Create Homebrew Tap Repository

Create a new public GitHub repository named `homebrew-tap`:
- Go to https://github.com/new
- Repository name: `homebrew-tap`
- Make it **public**
- Don't initialize with README

### 2. Create GitHub Personal Access Token

Create a fine-grained personal access token for Homebrew tap access:
- Go to https://github.com/settings/tokens?type=beta
- Click "Generate new token"
- Give it a name like `hardcover-cli-homebrew`
- Set expiration as needed
- Under "Repository access", select "Only select repositories" and choose your `homebrew-tap` repo
- Under "Permissions" → "Repository permissions", set:
  - **Contents**: Read and write
  - **Metadata**: Read-only
- Copy the token

### 3. Add GitHub Secret

Add the token as a secret to the `hardcover-cli` repository:
- Go to https://github.com/KIRKR101/hardcover-cli/settings/secrets/actions
- Click "New repository secret"
- Name: `HOMEBREW_TAP_TOKEN`
- Value: Paste the token from step 2
- Click "Add secret"

## Releasing

To create a new release:

1. Update the version in `cmd/root.go` (the `Version` variable)
2. Commit the change
3. Create and push a tag:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

The GitHub Actions workflow will automatically:
- Build binaries for macOS (arm64/amd64), Linux (amd64/arm64), and Windows (amd64)
- Create a GitHub release with the binaries
- Update the Homebrew formula in your `homebrew-tap` repository

## Testing Locally

To test the release process locally:

```bash
# Install GoReleaser
brew install goreleaser

# Build without publishing
goreleaser build --snapshot --clean

# Binaries will be in dist/
```

## User Installation

After the first release, users can install via:

```bash
# Homebrew
brew install KIRKR101/tap/hardcover

# Install script
curl -sSfL https://raw.githubusercontent.com/KIRKR101/hardcover-cli/main/install.sh | sh
```
