#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: generate-homebrew-formula.sh vX.Y.Z archive-directory [output-file]}"
archive_dir="${2:?usage: generate-homebrew-formula.sh vX.Y.Z archive-directory [output-file]}"
output="${3:-Formula/agstudio.rb}"
version="${tag#v}"

[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || {
  echo "Invalid release tag: $tag" >&2
  exit 2
}

sha256() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  else
    shasum -a 256 "$file" | awk '{print $1}'
  fi
}

darwin_arm64_sha="$(sha256 "$archive_dir/agstudio-${tag}-darwin-arm64.tar.gz")"
darwin_amd64_sha="$(sha256 "$archive_dir/agstudio-${tag}-darwin-amd64.tar.gz")"
linux_arm64_sha="$(sha256 "$archive_dir/agstudio-${tag}-linux-arm64.tar.gz")"
linux_amd64_sha="$(sha256 "$archive_dir/agstudio-${tag}-linux-amd64.tar.gz")"
release_url="https://github.com/herrmann13/agent_studio/releases/download/${tag}"

mkdir -p "$(dirname "$output")"
cat > "$output" <<EOF
class Agstudio < Formula
  desc "Terminal interface for Agent Studio"
  homepage "https://github.com/herrmann13/agent_studio"
  version "$version"
  license "MIT"

  on_macos do
    on_arm do
      url "$release_url/agstudio-${tag}-darwin-arm64.tar.gz"
      sha256 "$darwin_arm64_sha"
    end
    on_intel do
      url "$release_url/agstudio-${tag}-darwin-amd64.tar.gz"
      sha256 "$darwin_amd64_sha"
    end
  end

  on_linux do
    on_arm do
      url "$release_url/agstudio-${tag}-linux-arm64.tar.gz"
      sha256 "$linux_arm64_sha"
    end
    on_intel do
      url "$release_url/agstudio-${tag}-linux-amd64.tar.gz"
      sha256 "$linux_amd64_sha"
    end
  end

  def install
    bin.install "agstudio"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/agstudio --version")
  end
end
EOF
