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
  } @ inputs: let
    version =
      if (self ? shortRev)
      then self.shortRev
      else "dev";
  in
    flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};
      in rec {
        packages.kvdb = pkgs.buildGoModule {
          pname = "kvdb";
          inherit version;
          src = pkgs.lib.cleanSource self;
          checkFlags = ["-short"];
          vendorHash = "sha256-sKmqA7EcreNjgsECB1h63oBN4xfeDODa4x6/G/0HDxU=";
          subPackages = ["cmd/kvdb-server"];
          ldflags = ["-s" "-w"];
          passthru.exePath = "/bin/kvdb-server";
        };
        # nix build
        packages.default = packages.kvdb;
        apps.kvdb = flake-utils.lib.mkApp {
          drv = packages.kvdb;
        };
        # nix run
        apps.default = apps.kvdb;

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

        # nix flake check
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

        # nix fmt
        formatter = pkgs.alejandra;
      }
    );
}
