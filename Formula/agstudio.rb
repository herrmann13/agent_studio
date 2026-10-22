class Agstudio < Formula
  desc "Terminal interface for Agent Studio"
  homepage "https://github.com/herrmann13/agent_studio"
  version "0.2.3"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/herrmann13/agent_studio/releases/download/v0.2.3/agstudio-v0.2.3-darwin-arm64.tar.gz"
      sha256 "63026625c58f404f03f3baf48326d8ef11923b96844716d64c38c7eeba4b3a70"
    end
    on_intel do
      url "https://github.com/herrmann13/agent_studio/releases/download/v0.2.3/agstudio-v0.2.3-darwin-amd64.tar.gz"
      sha256 "61e0dd613dfde3785e4fdba72db194f9a86591c5f537f9f4afd9da5e1e3caaeb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/herrmann13/agent_studio/releases/download/v0.2.3/agstudio-v0.2.3-linux-arm64.tar.gz"
      sha256 "71d8d9c96ec295a474db765b34cc6fc3c1c9571697f71ff6f198be8861af5a75"
    end
    on_intel do
      url "https://github.com/herrmann13/agent_studio/releases/download/v0.2.3/agstudio-v0.2.3-linux-amd64.tar.gz"
      sha256 "52ecb9e01aba3ac24d7dfa68b4a7f004b8f901a887d04b7d0af803a3aa1dafb2"
    end
  end

  def install
    bin.install "agstudio"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/agstudio --version")
  end
end
