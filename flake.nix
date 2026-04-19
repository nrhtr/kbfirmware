{
  description = "kbfirmware — keyboard PCB firmware search";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        kbfirmware = pkgs.buildGoModule {
          pname = "kbfirmware";
          version = "0.1.0";

          src = pkgs.lib.cleanSourceWith {
            src = ./.;
            # Combine the standard cleanSourceFilter (excludes result symlinks,
            # .git, *.nix, .#* emacs lock files) with our DB exclusions.
            filter = path: type:
              let name = baseNameOf path; in
              pkgs.lib.cleanSourceFilter path type &&
              !(pkgs.lib.hasSuffix ".db" name) &&
              !(pkgs.lib.hasSuffix ".db-shm" name) &&
              !(pkgs.lib.hasSuffix ".db-wal" name);
          };

          # Update this hash after any change to go.mod / go.sum:
          #   nix build 2>&1 | grep "got:" | awk '{print $2}'
          vendorHash = "sha256-vlGmqYWcL5wC9c0JxzOjzCmSR+ju/lxscx2KBe9N2Fo=";

          # modernc.org/sqlite is pure Go — no CGo needed.
          env.CGO_ENABLED = "0";

          ldflags = [ "-s" "-w" ];
        };

        # Minimal layered Docker image. Timezone data is embedded in the binary
        # via time/tzdata, so no tzdata package is needed at runtime.
        dockerImage = pkgs.dockerTools.buildLayeredImage {
          name = "kbfirmware";
          tag = "latest";

          contents = [
            kbfirmware
            pkgs.cacert        # TLS roots for SMTP over TLS
          ];

          config = {
            Cmd = [ "/bin/kbfirmware" ];
            ExposedPorts."8080/tcp" = { };
            Env = [
              "DB_PATH=/data/kbfirmware.db"
              "LISTEN_ADDR=:8080"
              "SSL_CERT_FILE=/etc/ssl/certs/ca-bundle.crt"
            ];
            # Mount a persistent volume here for the SQLite DB.
            Volumes."/data" = { };
            WorkingDir = "/data";
          };
        };

      in
      {
        packages = {
          default = kbfirmware;
          docker = dockerImage;
        };

        # Load the docker image: nix build .#docker && docker load < result
        # Run the binary:        nix run
        # Enter dev shell:       nix develop

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            goose   # CLI for manual migration management: goose -db sqlite3 kbfirmware.db status
          ];
        };
      });
}
