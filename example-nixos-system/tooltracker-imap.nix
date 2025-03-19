{
  config,
  domain,
  lib,
  pkgs,
  ...
}:
let
  pizauth = lib.getExe pkgs.pizauth;
  logger = "${pkgs.util-linux}/bin/logger";
  cfg = config.services.tooltracker;
in
{
  environment.systemPackages = [
    config.services.tooltracker.package
    pkgs.pizauth
  ];

  users.users.tooltracker = {
    isSystemUser = true;
    group = "tooltracker";
  };
  users.groups.tooltracker = { };

  services = {
    tooltracker = {
      inherit domain;
      enable = true;
      listen = "0.0.0.0";
      http-port = 80;
      service = {
        user = "tooltracker";
        group = "tooltracker";
      };
      imap = {
        enable = true;
        host = "outlook.office365.com:993";
        user = "robertkovacsics@carallon.com";
        mailbox = "tooltracker";
        token-cmd = [
          pizauth
          "show"
          "tooltracker"
        ];
      };
      from = ".*";
    };
  };

  networking.firewall.allowedTCPPorts = [ 80 ];

  systemd.services.pizauth =
    let
      config = pkgs.writeText "pizauth.conf" ''
        auth_notify_cmd = "${logger} -t tooltracker 'Visit $PIZAUTH_URL'";

        // TODO (end user): Create new Entra token if you want a custom redirect URI
        account "tooltracker" {
            auth_uri = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize";
            token_uri = "https://login.microsoftonline.com/common/oauth2/v2.0/token";
            client_id = "0078db0d-6530-4bb8-bc8a-f54172d0aa39";
            scopes = [
              "https://outlook.office365.com/IMAP.AccessAsUser.All",
              "offline_access"
            ];
            auth_uri_fields = { "login_hint": "${cfg.imap.user}" };
            redirect_uri = "http://localhost";
        }
      '';
    in
    {
      description = "Pizauth OAuth2 token manager";

      wantedBy = [ "tooltracker.service" ];
      before = [ "tooltracker.service" ];
      serviceConfig = {
        User = "tooltracker";
        Group = "tooltracker";
        ExecStart = "${pizauth} server -d -vvvv -c ${config}";
        ExecReload = "${pizauth} reload";
        ExecStop = "${pizauth} shutdown";
      };
    };
}
