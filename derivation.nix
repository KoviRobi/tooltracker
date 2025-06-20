{
  lib,
  buildGoModule,
  unixODBC ? null,
  sqlite,
  withODBC ? false,
  src,
  version,
}:
assert withODBC -> unixODBC != null;
buildGoModule {
  pname = "tooltracker";
  inherit src version;

  buildInputs = if withODBC then [ unixODBC ] else [ sqlite ];

  tags = lib.optional withODBC "odbc";

  vendorHash = "sha256-niqK7St2gz+VeJFEnNF+uhNE4b5W0pZ/m4YObXeO3DA=";

  subPackages = [ "cmd/tooltracker" ];

  # To speed up build -- tests are more for development than packaging
  doCheck = false;

  meta.mainProgram = "tooltracker";
}
