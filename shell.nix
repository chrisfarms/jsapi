let
    pkgs = import <nixpkgs> {};
    stdenv = pkgs.stdenv;
in 
{
    goEnv = stdenv.mkDerivation rec {
        name = "shell";
        version = "shell";
        src = ./.;
        buildInputs = [ 
            pkgs.pkgconfig
            pkgs.go_1_3
            pkgs.autoconf213
            pkgs.perl
            pkgs.python
            pkgs.zip
            pkgs.git
        ];
        shellHook = ''
            export GOPATH=$(readlink -f ~)
        '';
    };
}
