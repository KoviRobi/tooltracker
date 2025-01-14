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

  boot.loader.grub = {
    enable = true;
    device = "nodev";
  };

  services = {
    tooltracker = {
      inherit domain;
      enable = true;
      listen = "0.0.0.0";
      smtpPort = 25;
      from = ".*";
    };

    httpd = {
      enable = true;

      virtualHosts.${domain} = {
        locations."/" = {
          proxyPass = with config.services.tooltracker; "http://${listen}:${toString httpPort}/";
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

}
