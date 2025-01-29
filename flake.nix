{
  inputs = {
    flake-parts.url = "github:hercules-ci/flake-parts";
    nixpkgs.url = "github:NixOS/nixpkgs";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    nixos-anywhere = {
      url = "github:nix-community/nixos-anywhere";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.disko.follows = "disko";
    };
    deploy-rs = {
      url = "github:serokell/deploy-rs";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    inputs@{
      nixpkgs,
      flake-parts,
      deploy-rs,
      nixos-anywhere,
      nixos-generators,
      ...
    }:
    flake-parts.lib.mkFlake { inherit inputs; } (
      {
        flake-parts-lib,
        self,
        withSystem,
        ...
      }:
      let
        inherit (flake-parts-lib) importApply;
      in
      {

        imports = [
          ./example-nixos-system
        ];

        systems = [
          "x86_64-linux"
          "x86_64-darwin"
          "aarch64-linux"
          "aarch64-darwin"
        ];

        perSystem =
          {
            self',
            config,
            lib,
            pkgs,
            ...
          }:
          {
            devShells.default = pkgs.mkShell {
              packages = [
                pkgs.go
                pkgs.gopls
                pkgs.sqlite
                pkgs.unixODBC
                pkgs.ansible
                pkgs.deploy-rs
                nixos-anywhere.packages.${pkgs.system}.default
                nixos-generators.packages.${pkgs.system}.default
              ];
            };

            packages = {
              tooltracker = pkgs.callPackage ./derivation.nix {
                version = self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";
                src = lib.cleanSource self;
              };
              tooltracker_sqlite = self'.packages.tooltracker;
              tooltracker_odbc = self'.packages.tooltracker.override {
                inherit (pkgs) unixODBC;
                withODBC = true;
              };
              default = self'.packages.tooltracker;
            };
          };

        flake.nixosModules = {
          default = self.nixosModules.tooltracker;
          tooltracker = importApply ./nixos-module.nix {
            localFlake = self;
            inherit withSystem;
          };
        };
      }
    );
}
