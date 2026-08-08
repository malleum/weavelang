{
  description = "weave — a strict functional language for Advent of Code, compiled through C";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      # Kept dependency-free on purpose: no flake-utils, so `nix build` needs
      # nothing but nixpkgs.
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      # The system name is passed explicitly rather than read back off pkgs,
      # since `pkgs.system` is deprecated in favour of
      # `pkgs.stdenv.hostPlatform.system`.
      forAllSystems = f:
        nixpkgs.lib.genAttrs systems
          (system: f system nixpkgs.legacyPackages.${system});

      version = if self ? shortRev then self.shortRev else "dev";
    in
    {
      packages = forAllSystems (system: pkgs: rec {
        weave = pkgs.buildGoModule {
          pname = "weave";
          inherit version;
          src = ./.;

          # The compiler has no third-party Go dependencies.
          vendorHash = null;

          subPackages = [ "cmd/weave" ];

          ldflags = [ "-s" "-w" "-X" "main.version=${version}" ];

          nativeBuildInputs = [ pkgs.makeWrapper ];

          # `weave build` shells out to a C compiler, so pin clang onto the
          # wrapper's PATH rather than trusting whatever the user happens to
          # have installed.
          postInstall = ''
            wrapProgram $out/bin/weave \
              --prefix PATH : ${nixpkgs.lib.makeBinPath [ pkgs.clang ]}
          '';

          meta = with nixpkgs.lib; {
            description = "The Weave language compiler";
            homepage = "https://github.com/malleum/weave";
            license = licenses.mit;
            mainProgram = "weave";
            platforms = platforms.unix;
          };
        };

        # The tree-sitter grammar, built the way nixpkgs builds every other
        # one, so `pkgs.vimPlugins.nvim-treesitter.withPlugins` and nixvim's
        # `grammarPackages` can take it directly.
        #
        # `src/parser.c` is committed, so this needs no tree-sitter CLI: it is
        # the same two translation units the editor would compile.
        tree-sitter-weave = pkgs.tree-sitter.buildGrammar {
          language = "weave";
          inherit version;
          src = ./tree-sitter-weave;
          meta.description = "Weave grammar for tree-sitter";
        };

        # The Neovim plugin, as a vim plugin derivation, so nixvim's
        # `extraPlugins` and lazy.nvim's `dir` can both take it.
        weave-nvim = pkgs.vimUtils.buildVimPlugin {
          pname = "weave.nvim";
          inherit version;
          src = ./weave.nvim;
          meta.description = "Weave in Neovim: every definition's value beside its line";
        };

        default = weave;
      });

      apps = forAllSystems (system: pkgs: rec {
        weave = {
          type = "app";
          program = "${self.packages.${system}.weave}/bin/weave";
        };
        default = weave;
      });

      devShells = forAllSystems (system: pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            go-tools # staticcheck
            clang
            gnumake
            tree-sitter # regenerating and testing the grammar
            nodejs # tree-sitter generate runs grammar.js
          ];

          shellHook = ''
            echo "weave dev shell — go $(go version | cut -d' ' -f3), $(clang --version | head -1)"
          '';
        };
      });

      checks = forAllSystems (system: pkgs: {
        inherit (self.packages.${system}) weave tree-sitter-weave weave-nvim;

        tests = pkgs.stdenv.mkDerivation {
          name = "weave-tests";
          src = ./.;
          nativeBuildInputs = [ pkgs.go pkgs.clang ];
          # The Go toolchain wants writable HOME and cache directories.
          buildPhase = ''
            export HOME=$TMPDIR
            export GOFLAGS=-mod=mod
            export GOCACHE=$TMPDIR/go-cache
            go test ./...
          '';
          installPhase = "touch $out";
        };
      });

      formatter = forAllSystems (system: pkgs: pkgs.nixpkgs-fmt);
    };
}
