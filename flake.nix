{
  description = "Playground Development Environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      nixpkgs,
      utils,
      ...
    }:
    utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        projectJdk = pkgs.jdk21;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # go
            go
            gofumpt
            golangci-lint
            goose
            gopls
            gotestsum
            sqlc

            # java
            projectJdk
            maven

            # fe
            nodejs_24

            # sql
            sqlfluff

            # infra
            localstack
            natscli
            tenv
            terraform-local
          ];

          JAVA_HOME = "${projectJdk}/lib/openjdk";
        };
      }
    );
}
