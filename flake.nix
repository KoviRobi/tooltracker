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
  };

  outputs =
    inputs@{
      self,
      nixpkgs,
      flake-parts,
      deploy-rs,
      nixos-anywhere,
      ...
    }:
    flake-parts.lib.mkFlake { inherit inputs; } {

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
        { config, pkgs, ... }:
        {
          devShells.default = pkgs.mkShell {
            packages = [
              pkgs.go
              pkgs.gopls
              pkgs.unixODBC
              pkgs.ansible
              pkgs.deploy-rs
              pkgs.nixos-anywhere
            ];
          };

          packages.default = self.packages.${pkgs.system}.tooltracker;
          packages.tooltracker = pkgs.buildGoModule {
            pname = "tooltracker";
            version = self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";

            src = "${self}";

            buildInputs = [ pkgs.unixODBC ];

            vendorHash = "sha256-J8spY6JsiaAb0Psm1HvWeLz2eryb/iRL3IWE+pEJHVI=";

            subPackages = [ "cmd/tooltracker/tooltracker.go" ];

            # To speed up build -- tests are more for development than packaging
            doCheck = false;
          };
        };

      flake.nixosModules.tooltracker =
        {
          pkgs,
          config,
          lib,
          ...
        }:
        let
          inherit (lib)
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

              package = mkPackageOption self.packages.${pkgs.system} "tooltracker" { };

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

              httpPrefix = mkOption {
                type = types.str;
                default = "";
                description = "tooltracker HTTP prefix (default \"\", i.e. root)";
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
                default = "Driver=SQLite;Database=tooltracker.db";
                description = "Unix ODBC path";
              };

              dkim = mkOption {
                type = types.str;
                default = "";
                description = "name of domain to check for DKIM signature";
              };

              smtpSend = mkOption {
                type = types.str;
                default = "";
                description = "SMTP server for sending mail";
              };

              smtpUser = mkOption {
                type = types.str;
                default = "";
                description = "user to log-in to send the SMTP server";
              };

              smtpPass = mkOption {
                type = types.str;
                default = "";
                description = "password to log-in to send the SMTP server";
              };
            };
          };

          config = mkIf cfg.enable {
            systemd.services.tooltracker = {
              description = "Tooltracker";

              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              script = ''
                ${cfg.package}/bin/tooltracker ${
                  lib.cli.toGNUCommandLineShell { } {
                    inherit (cfg)
                      listen
                      domain
                      from
                      to
                      dkim
                      ;

                    db = cfg.dbPath;
                    smtp = cfg.smtpPort;
                    http = cfg.httpPort;
                    http-prefix = cfg.httpPrefix;
                    send = cfg.smtpSend;
                    user = cfg.smtpUser;
                    pass = cfg.smtpPass;
                  }
                }
              '';

              serviceConfig = {
                AmbientCapabilities = mkIf (cfg.smtpPort < 1024 || cfg.httpPort < 1024) [
                  "CAP_NET_BIND_SERVICE"
                ];
                StateDirectory = "tooltracker";
                WorkingDirectory = "%S/tooltracker";
                DynamicUser = true;
                User = "tooltracker";
                Group = "tooltracker";
              };
            };
          };
        };
    };
}
