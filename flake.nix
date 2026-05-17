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
        moonOverlay = (
          final: prev: {
            moon = prev.moon.overrideAttrs (
              finalAttrs: previousAttrs: rec {
                version = "2.2.3";

                src = final.fetchFromGitHub {
                  owner = "moonrepo";
                  repo = "moon";
                  rev = "v${finalAttrs.version}";
                  hash = "sha256-DLZ4JNz3NlcZtlEn6kHYRym85lxLQ7TsQAhisrjqk98=";
                };

                cargoDeps = final.rustPlatform.fetchCargoVendor {
                  inherit src;
                  hash = "sha256-vWsUThZUzDD+K83DIou4xIsiSVp9FRm6dl8QrYD26Oc=";
                };

                nativeBuildInputs = (previousAttrs.nativeBuildInputs or [ ]) ++ [
                  final.protobuf
                ];
              }
            );
          }
        );
        pkgs = import nixpkgs {
          inherit system;
          overlays = [ moonOverlay ];
        };
        projectJdk = pkgs.jdk25;
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
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
