{
  description = "rss-tools";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs";
  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in {
      packages = forAllSystems (pkgs:
        let version = self.rev or "dev";
        in {
          default = self.packages.${pkgs.stdenv.hostPlatform.system}.rss-tools;
          rss-tools = pkgs.buildGo126Module {
            pname = "rss-tools";
            version = version;
            src = ./.;
            vendorHash = "sha256-HxCJvHCIve83OMvInOZtgcveC9uVZ5YAZAi5cL26akI=";
            ldflags = [ "-s" "-w" ];
            meta = with pkgs.lib; { license = licenses.mit; };
          };
        }
      );

      nixosModules.default = { config, lib, pkgs, ... }:
        with lib;
        let cfg = config.services.rss-tools;
        in {
          options.services.rss-tools = {
            enable = mkEnableOption "rss-tools service";

            user = mkOption {
              type = types.str;
              default = "rss-tools";
              description = "User account under which rss-tools runs.";
            };

            group = mkOption {
              type = types.str;
              default = "rss-tools";
              description = "Group under which rss-tools runs.";
            };

            package = mkOption {
              type = types.package;
              default = self.packages.${pkgs.stdenv.hostPlatform.system}.rss-tools;
              defaultText = literalExpression "self.packages.\${pkgs.stdenv.hostPlatform.system}.rss-tools";
              description = "rss-tools package to run.";
            };

            settingsFile = mkOption {
              type = types.nullOr types.str;
              default = null;
              example = "/run/secrets/rss-tools.json";
              description = "Path to JSON config file passed to --config.";
            };

            dbPath = mkOption {
              type = types.str;
              default = "%S/rss-tools/db";
              example = "/var/lib/rss-tools/rss.db";
              description = "Path to local bbolt DB file passed to --db.";
            };
          };

          config = mkIf cfg.enable {
            assertions = [
              {
                assertion = cfg.settingsFile != null;
                message = "services.rss-tools.settingsFile must be set";
              }
            ];

            users.groups.${cfg.group} = { };
            users.users.${cfg.user} = {
              isSystemUser = true;
              group = cfg.group;
            };

            systemd.services.rss-tools = {
              description = "rss-tools service";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];
              serviceConfig = {
                Type = "simple";
                User = cfg.user;
                Group = cfg.group;
                StateDirectory = "rss-tools";
                WorkingDirectory = "%S/rss-tools";
                ExecStart = "${cfg.package}/bin/rss-tools --config ${cfg.settingsFile} --db ${cfg.dbPath}";
                Restart = "on-failure";
                RestartSec = "5s";
                NoNewPrivileges = true;
                PrivateTmp = true;
                ProtectSystem = "strict";
                ProtectHome = true;
              };
            };
          };
        };
    };
}
