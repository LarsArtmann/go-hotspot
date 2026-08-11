{
  description = "go-hotspot — code complexity × churn hotspot analysis";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        go-hotspot = pkgs.buildGoModule {
          pname = "go-hotspot";
          version = "0.2.0";
          src = ./.;
          subPackages = [ "cmd/go-hotspot" ];
          vendorHash = "sha256-p/sUFZTQXgA32HgsV1udsTO3J3PDC7iGsDeKm+/nLe8=";
          env = {
            CGO_ENABLED = 0;
            GOEXPERIMENT = "jsonv2";
          };
        };
      in
      {
        packages.default = go-hotspot;

        apps = {
          build = {
            type = "app";
            program = toString (pkgs.writeShellScript "build" ''
              export GOEXPERIMENT=jsonv2
              ${pkgs.go}/bin/go build ./cmd/go-hotspot
            '');
          };

          test = {
            type = "app";
            program = toString (pkgs.writeShellScript "test" ''
              export GOEXPERIMENT=jsonv2
              ${pkgs.go}/bin/go test ./... -race -gcflags=all=-l
            '');
          };

          lint = {
            type = "app";
            program = toString (pkgs.writeShellScript "lint" ''
              export GOEXPERIMENT=jsonv2
              ${pkgs.golangci-lint}/bin/golangci-lint run ./...
            '');
          };

          format = {
            type = "app";
            program = toString (pkgs.writeShellScript "format" ''
              ${pkgs.gofumpt}/bin/gofumpt -w .
            '');
          };

          vet = {
            type = "app";
            program = toString (pkgs.writeShellScript "vet" ''
              export GOEXPERIMENT=jsonv2
              ${pkgs.go}/bin/go vet ./...
            '');
          };
        };

        devShells.default = pkgs.mkShell {
          GOEXPERIMENT = "jsonv2";
          buildInputs = with pkgs; [
            go
            golangci-lint
            gofumpt
            goreleaser
          ];
        };
      });
}
