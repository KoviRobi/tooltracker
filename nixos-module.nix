{ localFlake, withSystem, ... }:
{
  config,
  lib,
  options,
  pkgs,
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
  cfg = config.services.tooltracker;
  opt = options.services.tooltracker;
in
{
  options = {
    services.tooltracker = {
      enable = mkEnableOption "Tooltracker service";

      package = withSystem pkgs.system (
        { config, ... }: mkPackageOption config.packages "tooltracker" { }
      );

      listen = mkOption {
        type = types.str;
        default = "localhost";
        description = "Host name/IP to listen on";
      };

      domain = mkOption {
        type = types.nullOr types.str;
        default = config.networking.hostName;
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
        type = types.nullOr types.str;
        default = null;
        description = "tooltracker HTTP prefix";
      };

      from = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "regex for emails which are not anonimised";
      };

      to = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "name of mailbox to send mail to";
      };

      db = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "SQLite3 path or Unix ODBC path (depending on build flag)";
      };
      dkim = mkOption {
        type = types.nullOr types.str;
        default = null;
        description = "name of domain to check for DKIM signature";
      };

      delegate = mkOption {
        type = types.nullOr types.bool;
        default = null;
        description = ''
          Whether to enable users to delegate to personal emails (only
          meaningful if DKIM is used)
        '';
      };

      local-dkim = mkOption {
        type = types.nullOr types.bool;
        default = null;
        description = ''
          Whether to enable "DKIM on mails within the domain (some services
          don't sign internal mail)
        '';
      };

      service = {
        user = mkOption {
          type = types.str;
          default = "tooltracker-dyn";
          description = ''
            The user the tooltracker service runs under. If left default then
            it uses systemd's DynamicUser.
          '';
        };

        group = mkOption {
          type = types.str;
          default = "tooltracker-dyn";
          description = ''
            The group the tooltracker service runs under.
          '';
        };
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
          type = types.nullOr types.str;
          default = null;
          example = "someuser@domain.org";
          description = "User to log in as";
        };

        mailbox = mkOption {
          type = types.nullOr types.str;
          default = null;
          example = "tooltracker";
          description = "mailbox to monitor";
        };

        token-cmd = mkOption {
          type = types.listOf types.str;
          default = [ ];
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
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];

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
        DynamicUser = cfg.service.user == opt.service.user.default;
        User = cfg.service.user;
        Group = cfg.service.group;
      };
    };

    assertions = [
      {
        assertion = cfg.imap.enable != cfg.smtp.enable;
        message = "Only one of IMAP and SMTP should be enabled";
      }
    ];
  };
}
