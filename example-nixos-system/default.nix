{
  self,
  inputs,
  ...
}:
{
  flake =
    let
      system = "x86_64-linux";
    in
    {
      nixosConfigurations.example = inputs.nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          self.nixosModules.tooltracker

          (
            { modulesPath, ... }:
            {
              # Add deployed version metadata
              system.configurationRevision =
                self.rev or self.dirtyRev or "${builtins.substring 0 8 self.lastModifiedDate}-dirty";
            }
          )

          ./configuration.nix
          ./hardware-configuration.nix
        ];
      };

      deploy.nodes.aws = {
        hostname = "aws";
        user = "root";
        profiles.system = {
          path = inputs.deploy-rs.lib.${system}.activate.nixos self.nixosConfigurations.example;
        };
      };
    };
}
