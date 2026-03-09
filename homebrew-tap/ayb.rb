class Ayb < Formula
  desc "Backend-as-a-Service for PostgreSQL. Single binary, one config file."
  homepage "https://github.com/gridlhq-staging/allyourbase"
  version "0.0.12-beta"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/gridlhq-staging/allyourbase/releases/download/v#{version}/ayb_#{version}_darwin_arm64.tar.gz"
      sha256 "456dca70b2604386de794988474a50d8104d96a8b47b7721453bd8a5a88efde5"
    end
    on_intel do
      url "https://github.com/gridlhq-staging/allyourbase/releases/download/v#{version}/ayb_#{version}_darwin_amd64.tar.gz"
      sha256 "8a9b1a80a6adc8aadddd7e82d2511647a0dd060e4253298d0663d8c8a83643f1"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/gridlhq-staging/allyourbase/releases/download/v#{version}/ayb_#{version}_linux_arm64.tar.gz"
      sha256 "aed5ee9c9015bc6c2e04ab0d801ebdd0504593e7f26eb26f3b725bcef960efcf"
    end
    on_intel do
      url "https://github.com/gridlhq-staging/allyourbase/releases/download/v#{version}/ayb_#{version}_linux_amd64.tar.gz"
      sha256 "8c7aa2b4b43b18058d30f4c29aa419852903cdaf9c09c3061db9a048ae05a134"
    end
  end

  def install
    bin.install "ayb"
  end

  test do
    assert_match "ayb", shell_output("#{bin}/ayb version")
  end
end
