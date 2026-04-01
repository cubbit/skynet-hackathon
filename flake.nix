{
  description = "Skynet";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };
  };

  outputs = inputs@{ flake-parts, ... }:
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = [ "x86_64-linux" "aarch64-linux" "aarch64-darwin" "x86_64-darwin" ];

      perSystem = { pkgs, system, ... }:
        let
          chdsh = pkgs.writeScriptBin "chdsh" ''
            echo "export DEVSHELL=$@" > .envrc.user
            direnv reload
          '';
        in
        {
          devShells.default = pkgs.mkShell {
            packages = with pkgs; [
              # nix
              nixpkgs-fmt

              # python
              python3
              basedpyright

              rclone

              # misc
              parallel
              openssl
              pkg-config
            ] ++ lib.optionals (lib.hasSuffix "darwin" system) [
              # darwin
            ];
            buildInputs = [ pkgs.bashInteractive ];
          };
        };
    };
}
