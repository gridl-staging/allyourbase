{ lib, buildGoModule, fetchFromGitHub }:

buildGoModule rec {
  pname = "ayb";
  version = "0.0.0";

  src = fetchFromGitHub {
    owner = "gridlhq";
    repo = "allyourbase";
    rev = "v${version}";
    hash = "sha256-REPLACE_WITH_SOURCE_HASH";
  };

  vendorHash = "sha256-REPLACE_WITH_VENDOR_HASH";

  subPackages = [ "cmd/ayb" ];

  ldflags = [
    "-s"
    "-w"
    "-X"
    "main.version=${version}"
  ];

  env.CGO_ENABLED = 0;

  meta = with lib; {
    description = "Backend-as-a-Service for PostgreSQL. Single binary, one config file.";
    homepage = "https://github.com/gridlhq/allyourbase";
    license = licenses.mit;
    maintainers = [ ];
    platforms = platforms.unix;
    mainProgram = "ayb";
  };

  # Flake usage pattern:
  # packages.${system}.ayb = pkgs.callPackage ./default.nix { };
}
