{
  inputs,
  self,
  ...
}:
{
  flake =
    let
      system = "x86_64-linux";
      inherit (inputs.nixpkgs) lib;
    in
    {
      nixosConfigurations.example = lib.nixosSystem {
        inherit system;

        # This makes `domain` passed to modules
        specialArgs = {
          domain = "tooltracker-proto.co.uk";
        };

        modules = [
          self.nixosModules.tooltracker
          inputs.nixos-generators.nixosModules.all-formats

          (
            {
              config,
              domain,
              modulesPath,
              pkgs,
              ...
            }:
            {
              # Add deployed version metadata
              system.configurationRevision =
                self.rev or self.dirtyRev or "${builtins.substring 0 8 self.lastModifiedDate}-dirty";

              environment.systemPackages = [
                pkgs.curl
                pkgs.gitMinimal
                pkgs.vim
              ];

              users.users.root.openssh.authorizedKeys.keys = [
                # TODO: change this to your ssh key
                "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAILPZ0IUFFBr4jQtm91e2YiAnQwZSTfpKFukeRN2oZH2J TODO: CHANGEME"
              ];

              system.stateVersion = builtins.substring 0 5 lib.version;

              services = {
                httpd = {
                  enable = true;

                  virtualHosts.${domain} = {
                    locations."/" = {
                      proxyPass = with config.services.tooltracker; "http://${listen}:${toString http-port}/";
                    };
                  };
                };

                sshd.enable = true;
              };

              networking.firewall.allowedTCPPorts = [
                80
                443
              ];
            }
          )

          # ./tooltracker-smtp.nix
          ./tooltracker-imap.nix
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
