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
      self,
      nixpkgs,
      flake-parts,
      deploy-rs,
      nixos-anywhere,
      nixos-generators,
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
        {
          self',
          config,
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

          packages =
            let
              tooltracker =
                {
                  lib,
                  buildGoModule,
                  unixODBC ? null,
                  sqlite,
                  withODBC ? false,
                }:
                assert withODBC -> unixODBC != null;
                buildGoModule {
                  pname = "tooltracker";
                  version = self.rev or "unstable${builtins.substring 0 8 self.lastModifiedDate}";

                  src = "${self}";

                  buildInputs = if withODBC then [ unixODBC ] else [ sqlite ];

                  tags = lib.optional withODBC "odbc";

                  vendorHash = "sha256-nEH/ma3Md8B2hvdAxbESVE5pdHAfOHHUnWrNrky9cSw=";

                  subPackages = [ "cmd/tooltracker/tooltracker.go" ];

                  # To speed up build -- tests are more for development than packaging
                  doCheck = false;
                };
            in
            {
              tooltracker_sqlite = self'.packages.default;
              tooltracker_odbc = self'.packages.tooltracker_sqlite.override {
                inherit (pkgs) unixODBC;
                withODBC = true;
              };
              default = pkgs.callPackage tooltracker { };
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

              package = mkPackageOption self.packages.${pkgs.system} "default" { };

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

              smtp-port = mkOption {
                type = types.ints.u16;
                default = 1025;
                description = "Port for SMTP to listen on";
              };

              http-port = mkOption {
                type = types.ints.u16;
                default = 8123;
                description = "Port for HTTP to listen on";
              };

              http-prefix = mkOption {
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
                type = types.nullOr types.str;
                default = null;
                description = "SQLite3 path or Unix ODBC path (depending on build flag)";
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
                      http-port
                      http-prefix
                      smtp-port
                      ;

                    db = cfg.dbPath;
                    send = cfg.smtpSend;
                    user = cfg.smtpUser;
                    pass = cfg.smtpPass;
                  }
                }
              '';

              serviceConfig = {
                AmbientCapabilities = mkIf (cfg.smtp-port < 1024 || cfg.http-port < 1024) [
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
