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

  vendorHash = "sha256-vKXtxL43lE/tdsIR2iAIek2kkKtTDZVjnN0035YiwHg=";

  subPackages = [ "cmd/tooltracker" ];

  # To speed up build -- tests are more for development than packaging
  doCheck = false;

  meta.mainProgram = "tooltracker";
}
