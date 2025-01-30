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
      smtp = {
        enable = true;
        port = 25;
      };
      from = ".*";
    };
  };
}
