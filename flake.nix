{
  description = "Reimplementation of kvdb.io";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    pre-commit-hooks = {
      url = "github:cachix/pre-commit-hooks.nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
    pre-commit-hooks,
    ...
  } @ inputs:
    {
    }
    // flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go

            # dev-tools
            alejandra
            just
            nil
            taplo
            typos
          ];
          shellHook = self.checks.${system}.pre-commit-check.shellHook;
        };

        checks = {
          pre-commit-check = pre-commit-hooks.lib.${system}.run {
            src = ./.;
            hooks = {
              gofmt.enable = true;
              govet.enable = true;
              alejandra.enable = true;
              typos = {
                enable = true;
                settings.write = true;
              };
              taplo.enable = true;
            };
          };
        };

        formatter = pkgs.alejandra;
      }
    );
}
