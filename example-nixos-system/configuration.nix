{
  config,
  lib,
  pkgs,
  ...
}:
let
  domain = "tooltracker-proto.co.uk";
in
{
  services = {
    tooltracker = {
      inherit domain;
      enable = true;
      listen = "0.0.0.0";
      smtp = {
        enable = true;
        port = 25;
      };
      from = ".*";
    };

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

  environment.unixODBCDrivers = [ pkgs.unixODBCDrivers.sqlite ];

  networking.firewall.allowedTCPPorts = [
    80
    443
  ];
}
