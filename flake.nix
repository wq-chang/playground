{
  description = "Playground Development Environment";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
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
        pkgs = import nixpkgs {
          inherit system;
        };
        projectJdk = pkgs.jdk25;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            mermaid-cli
            jq
            moon
            graphviz # visualize mem allocation
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
            # localstack
            kafkactl
            tenv
            terraform-local
          ];

          JAVA_HOME = "${projectJdk}/lib/openjdk";
        };
      }
    );
}
