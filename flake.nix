{
  inputs.deploy-rs.url = "github:serokell/deploy-rs";
  inputs.deploy-rs.inputs.nixpkgs.follows = "nixpkgs";

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      deploy-rs,
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
            deploy-rs.packages.${system}.deploy-rs
          ];
        };

        packages.default = self.packages.${system}.tooltracker;
        packages.tooltracker = pkgs.buildGoModule {
          pname = "tooltracker";
          version = self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";

          src = "${self}";

          vendorHash = "sha256-ncuaR7JGJBEhXH285mG0V6oLlGul7jUyqT2+oVwWAuE=";

          subPackages = [ "cmd/tooltracker.go" ];

          # To speed up build -- tests are more for development than packaging
          doCheck = false;
        };

        nixosModules.tooltracker =
          { config, ... }:
          let
            inherit (pkgs.lib)
              mkOption
              mkEnableOption
              mkPackageOption
              mkIf
              types
              ;
            inherit (pkgs.lib.strings) escapeShellArg;
            cfg = config.services.tooltracker;
          in
          {
            options = {
              services.tooltracker = {
                enable = mkEnableOption "Tooltracker service";

                package = mkPackageOption self.packages.${system} "tooltracker" { };

                listen = mkOption {
                  type = types.str;
                  default = "localhost";
                  description = "Host name/IP to listen on";
                };

                domain = mkOption {
                  type = types.str;
                  default = "localhost";
                  description = ''
                    Host name/IP to respond to HELO/EHLO; usually public FQDN
                    or public IP. Also used for QR code.
                  '';
                };

                smtpPort = mkOption {
                  type = types.ints.u16;
                  default = 1025;
                  description = "Port for SMTP to listen on";
                };

                httpPort = mkOption {
                  type = types.ints.u16;
                  default = 8123;
                  description = "Port for HTTP to listen on";
                };

                from = mkOption {
                  type = types.str;
                  default = "^.*@work.com$";
                  description = "regex for emails which are not anonimised";
                };

                to = mkOption {
                  type = types.str;
                  default = "tooltracker";
                  description = "name of mailbox to send mail to";
                };

                dbPath = mkOption {
                  type = types.str;
                  default = "%S/tooltracker.db";
                  description = "path to sqlite3 file to create/use";
                };
              };
            };

            config = mkIf cfg.enable {
              systemd.services.tooltracker = {
                description = "Tooltracker";

                wantedBy = [ "multi-user.target" ];
                after = [ "network.target" ];

                script = ''
                  ${cfg.package}/bin/tooltracker \
                    -listen ${escapeShellArg cfg.listen} \
                    -domain ${escapeShellArg cfg.domain} \
                    -smtp ${toString cfg.smtpPort} \
                    -http ${toString cfg.httpPort} \
                    -from ${escapeShellArg cfg.from} \
                    -to ${escapeShellArg cfg.to} \
                    -db ${escapeShellArg cfg.dbPath}
                '';
              };
            };
          };
      }
    )
    // (
      let
        system = "x86_64-linux";
      in
      {
        nixosConfigurations.example = nixpkgs.lib.nixosSystem {
          modules = [
            self.nixosModules.${system}.tooltracker

            (
              {
                config,
                lib,
                pkgs,
                modulesPath,
                ...
              }:
              {
                system.stateVersion = builtins.substring 0 5 nixpkgs.lib.version;

                system.configurationRevision =
                  self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";

                boot.loader.grub.enable = true;
                boot.loader.grub.device = "nodev";
                boot.initrd.availableKernelModules = [
                  "nvme"
                  "ata_piix"
                  "xen_blkfront"
                ];
                boot.initrd.kernelModules = [ ];
                boot.kernelModules = [ ];
                boot.extraModulePackages = [ ];
                boot.kernelParams = [
                  "console=tty0"
                  "console=ttyS0,115200n8"
                  "boot.shell_on_fail"
                  # For some reason, EC2 takes a panful amount of time (~5 min)
                  # to power off a hung instance
                  "systemd.crash_action=poweroff"
                ];

                services.tooltracker.enable = true;
                services.tooltracker.listen = "0.0.0.0";
                # TODO: Customize to your own config
                services.tooltracker.domain = "tooltracker-proto.co.uk";
                services.tooltracker.smtpPort = 25;
                services.tooltracker.from = ".*";

                services.nginx = {
                  enable = true;
                  additionalModules = [ ];
                  recommendedProxySettings = true;

                  virtualHosts."tooltracker-proto.co.uk" = {
                    default = true;
                    enableACME = true;
                    addSSL = true;
                    locations."/" = {
                      proxyPass = with config.services.tooltracker; "http://${listen}:${toString httpPort}";
                    };
                  };
                };
                security.acme.acceptTerms = true;
                # TODO: Customize to your own config
                security.acme.certs."tooltracker-proto.co.uk".email = "kovirobi@gmail.com";
                # TODO: Customize to your own config
                nix.settings.trusted-public-keys = [
                  "promethium-nix1:GfINhsNRWatD91mYMa9VTTywjNctN6l2T2FUlNP7DLc="
                ];

                networking.firewall.allowedTCPPorts = [
                  25
                  80
                  443
                ];

                services.sshd.enable = true;

                users.users.ec2-user = {
                  isNormalUser = true;
                  # TODO: Customize to your own config
                  openssh.authorizedKeys.keyFiles = [ ./example.pub ];
                  extraGroups = [ "wheel" ];
                };
                security.sudo.defaultOptions = [
                  "SETENV"
                  "NOPASSWD"
                ];

                fileSystems."/" = {
                  device = "/dev/disk/by-uuid/693eea79-11af-44b1-9c1e-01aced209966";
                  fsType = "xfs";
                };

                fileSystems."/boot/efi" = {
                  device = "/dev/disk/by-uuid/A41B-D0D1";
                  fsType = "vfat";
                  options = [
                    "fmask=0077"
                    "dmask=0077"
                  ];
                };

                swapDevices = [ ];

                # Enables DHCP on each ethernet and wireless interface. In case of scripted networking
                # (the default) this is the recommended approach. When using systemd-networkd it's
                # still possible to use this option, but it's recommended to use it in conjunction
                # with explicit per-interface declarations with `networking.interfaces.<interface>.useDHCP`.
                networking.useDHCP = lib.mkDefault true;
                # networking.interfaces.enX0.useDHCP = lib.mkDefault true;

                nixpkgs.hostPlatform = lib.mkDefault system;
              }
            )
          ];
        };

        deploy.nodes.aws = {
          # TODO: Customize to your own config
          # Configured in ~/.ssh/config:
          # Host aws
          #   User ec2-user
          #   IdentityFile ~/.ssh/tooltracker.pem
          #   HostName 18.175.116.44
          hostname = "aws";
          sshUser = "ec2-user";
          user = "root";
          profiles.system = {
            path = deploy-rs.lib.${system}.activate.nixos self.nixosConfigurations.example;
          };
        };
      }
    );
}
