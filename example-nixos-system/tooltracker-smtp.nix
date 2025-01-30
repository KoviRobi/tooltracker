{
  domain,
  ...
}:
{
  services = {
    tooltracker = {
      inherit domain;
      enable = true;
      listen = "0.0.0.0";
      http-port = 80;
      smtp = {
        enable = true;
        port = 25;
      };
      from = ".*";
    };
  };

  networking.firewall.allowedTCPPorts = [ 80 ];
}
