{
  outputs =
    inputs@{
      self,
      nixpkgs,
      flake-parts,
    }:
    flake-parts.lib.mkFlake { inherit inputs; } {

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
              pkgs.sqlite
              pkgs.ansible
            ];
          };

          packages.default = self.packages.${pkgs.system}.tooltracker;
          packages.tooltracker = pkgs.buildGoModule {
            pname = "tooltracker";
            version = self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";

            src = "${self}";

            vendorHash = "sha256-NaBGR1GLnaR1fp+NUWAdcYFi08SWhB0s4mBmyY1yCnQ=";

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
                default = "tooltracker.db";
                description = "path to sqlite3 file to create/use";
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
