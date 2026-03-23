class Kc < Formula
  desc "Human-friendly macOS Keychain CLI"
  homepage "https://github.com/v-gutierrez/kc"
  url "https://github.com/v-gutierrez/kc/releases/download/v0.1.0/kc-darwin-arm64.tar.gz"
  sha256 "REPLACE_WITH_SHA256"
  license "MIT"

  def install
    bin.install "kc"
  end

  test do
    assert_match "kc", shell_output("#{bin}/kc --help")
  end
end
