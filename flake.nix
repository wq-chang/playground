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
            gnumake
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

          shellHook = ''
            if [ ! -d "node_modules" ]; then
                echo "📦 node_modules not found. Installing..."
                npm install
              fi
              export PATH="$PWD/node_modules/.bin:$PATH"
          '';
          JAVA_HOME = "${projectJdk}/lib/openjdk";
        };
      }
    );
}
