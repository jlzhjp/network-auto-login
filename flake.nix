{
  description = "Automatic captive portal login helper";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-linux"
        "x86_64-linux"
      ];

      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          buildGoModule = pkgs.buildGo125Module or pkgs.buildGoModule;
          package = buildGoModule {
            pname = "network-auto-login";
            version = "0.1.0";

            src = self;
            vendorHash = null;

            meta = {
              description = "Automatic network captive portal login helper";
              mainProgram = "network-auto-login";
              license = nixpkgs.lib.licenses.mit;
              platforms = nixpkgs.lib.platforms.linux;
            };
          };
        in
        {
          network-auto-login = package;
          default = package;
        }
      );

      checks = forAllSystems (system: {
        network-auto-login = self.packages.${system}.network-auto-login;
      });

      nixosModules.network-auto-login =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        let
          cfg = config.services.network-auto-login;
        in
        {
          options.services.network-auto-login = {
            enable = lib.mkEnableOption "automatic network captive portal login";

            package = lib.mkPackageOption self.packages.${pkgs.stdenv.hostPlatform.system} "network-auto-login" { };

            credentialsFile = lib.mkOption {
              type = lib.types.nullOr lib.types.path;
              default = null;
              description = ''
                Environment file containing NETWORK_LOGIN_USERNAME and
                NETWORK_LOGIN_PASSWORD.
              '';
            };

            fwmark = lib.mkOption {
              type = lib.types.str;
              default = "0x80000";
              description = "Firewall mark to apply to outbound login requests.";
            };

            onActiveSec = lib.mkOption {
              type = lib.types.str;
              default = "10sec";
              description = "Delay before the timer first starts the login service.";
            };

            onUnitActiveSec = lib.mkOption {
              type = lib.types.str;
              default = "10min";
              description = "Delay between repeated login service activations.";
            };
          };

          config = lib.mkIf cfg.enable {
            assertions = [
              {
                assertion = cfg.credentialsFile != null;
                message = "services.network-auto-login.credentialsFile must be set.";
              }
            ];

            systemd.services.network-auto-login = {
              description = "Network Auto Login Service";
              wants = [ "network-online.target" ];
              after = [ "network-online.target" ];

              serviceConfig = {
                Type = "oneshot";
                EnvironmentFile = cfg.credentialsFile;
                Environment = "NETWORK_LOGIN_FWMARK=${cfg.fwmark}";
                ExecStart = lib.getExe cfg.package;
              };
            };

            systemd.timers.network-auto-login = {
              description = "Timer for Network Auto Login Service";
              wantedBy = [ "timers.target" ];

              timerConfig = {
                OnActiveSec = cfg.onActiveSec;
                OnUnitActiveSec = cfg.onUnitActiveSec;
              };
            };
          };
        };

      nixosModules.default = self.nixosModules.network-auto-login;
    };
}
