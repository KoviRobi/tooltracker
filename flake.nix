{
  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.sqlite
          ];
        };

        packages.default = self.packages.${system}.tooltracker;
        packages.tooltracker = pkgs.buildGoModule {
          pname = "tooltracker";
          version = self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";

          src = "${self}";

          vendorHash = "sha256-ncuaR7JGJBEhXH285mG0V6oLlGul7jUyqT2+oVwWAuE=";

          subPackages = [ "cmd/tooltracker.go" ];
        };
      }
    );
}
