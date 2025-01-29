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

                  vendorHash = "sha256-vKXtxL43lE/tdsIR2iAIek2kkKtTDZVjnN0035YiwHg=";

                  subPackages = [ "cmd/tooltracker" ];

                  # To speed up build -- tests are more for development than packaging
                  doCheck = false;

                  meta.mainProgram = "tooltracker";
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

              db = mkOption {
                type = types.nullOr types.str;
                default = null;
                description = "SQLite3 path or Unix ODBC path (depending on build flag)";
              };
              dkim = mkOption {
                type = types.str;
                default = "";
                description = "name of domain to check for DKIM signature";
              };

              delegate =
                mkEnableOption "users to delegate to personal emails (only meaningful if DKIM is used)"
                // {
                  default = true;
                  example = false;
                };

              local-dkim =
                mkEnableOption "DKIM on mails within the domain (some services don't sign internal mail)"
                // {
                  default = true;
                  example = false;
                };

              smtp = {
                enable = mkEnableOption "using SMTP to receive mail. Mutually exclusive with IMAP.";

                port = mkOption {
                  type = types.ints.u16;
                  default = 1025;
                  description = "Port for SMTP to listen on";
                };
              };

              imap = {
                enable = mkEnableOption "using IMAP to receive mail. Mutually exclusive with SMTP.";

                idle-poll = mkOption {
                  type = types.nullOr types.str;
                  default = null;
                  example = "30m";
                  description = ''
                    restart IMAP IDLE after this amount of time. Takes a go
                    duration, i.e. number with a suffix of h/m/s
                  '';
                };

                host = mkOption {
                  type = types.nullOr types.str;
                  default = null;
                  example = "outlook.office365.com:993";
                  description = "Host, including port, to connect to";
                };

                user = mkOption {
                  type = types.str;
                  example = "someuser@domain.org";
                  description = "User to log in as";
                };

                mailbox = mkOption {
                  type = types.str;
                  default = "INBOX";
                  example = "tooltracker";
                  description = "mailbox to monitor";
                };

                token-cmd = mkOption {
                  type = types.listOf types.str;
                  example = ''
                    [ (lib.getExe pkgs.pizauth) "show" "tooltracker" ]
                  '';
                  description = "command to use to get the OAuth token";
                };
              };
            };
          };

          config = mkIf cfg.enable {
            systemd.services.tooltracker = {
              description = "Tooltracker";

              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              script =
                let
                  mainFlags = lib.cli.toGNUCommandLineShell { } {
                    inherit (cfg)
                      db
                      dkim
                      delegate
                      local-dkim
                      domain
                      from
                      http-port
                      http-prefix
                      listen
                      to
                      ;
                  };

                  command =
                    if cfg.smtp.enable then
                      "smtp ${
                        lib.cli.toGNUCommandLineShell { } {
                          smtp-port = cfg.smtp.port;
                        }
                      }"
                    else
                      "imap ${
                        lib.cli.toGNUCommandLineShell { } {
                          inherit (cfg.imap) idle-poll mailbox token-cmd;
                          imap-host = cfg.imap.host;
                          imap-user = cfg.imap.user;
                        }
                      }";
                in
                ''
                  ${lib.getExe cfg.package} ${mainFlags} ${command}
                '';

              serviceConfig = {
                AmbientCapabilities = mkIf (cfg.smtp.port < 1024 || cfg.http-port < 1024) [
                  "CAP_NET_BIND_SERVICE"
                ];
                StateDirectory = "tooltracker";
                WorkingDirectory = "%S/tooltracker";
                DynamicUser = true;
                User = "tooltracker";
                Group = "tooltracker";
              };
            };

            assertions = [
              {
                assertion = cfg.imap.enable != cfg.smtp.enable;
                message = "Only one of IMAP and SMTP should be enabled";
              }
            ];
          };
        };
    };
}
