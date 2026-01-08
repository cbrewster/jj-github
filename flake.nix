{
  inputs = {
    utils.url = "github:numtide/flake-utils";
  };
  outputs = { self, nixpkgs, utils }: utils.lib.eachDefaultSystem (system:
    let
      pkgs = nixpkgs.legacyPackages.${system};
    in
    {
      packages.default = pkgs.buildGoModule {
        pname = "jj-github";
        version = "0.1.0";
        src = ./.;
        vendorHash = "sha256-lmcuaQ2yj/4CBQW4lwf2oKRfO6ip4MhBnI85dNqdfIw=";

        meta = with pkgs.lib; {
          description = "Manage stacked pull requests with Jujutsu and GitHub";
          homepage = "https://github.com/cbrewster/jj-github";
          license = licenses.mit;
          mainProgram = "jj-github";
        };
      };

      devShells.default = pkgs.mkShell {
        buildInputs = with pkgs; [
          go
          gopls
        ];
      };
    }
  );
}
