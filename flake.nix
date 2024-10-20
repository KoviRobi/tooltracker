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

          vendorHash = "sha256-OAlkNI6CDPpq6CFfpbqQwJSzQCFK1iht8+gNckAmz7I=";

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
                  default = "tooltracker.db";
                  description = "path to sqlite3 file to create/use";
                };

                dkim = mkOption {
                  type = types.str;
                  default = "";
                  description = "name of domain to check for DKIM signature";
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
                    -db ${escapeShellArg cfg.dbPath} \
                    -dkim ${escapeShellArg cfg.dkim}
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
      }
    );
}
