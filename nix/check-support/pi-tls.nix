{ inputs, pkgs }:
let
  # Independent roots distinguish public-provider trust from extra broker trust.
  # These keys are test-only, generated in the build sandbox, and never used live.
  certificates = pkgs.runCommand "den-pi-tls-fixtures" { nativeBuildInputs = [ pkgs.openssl ]; } ''
    mkdir -p "$out/etc/ssl/certs"
    for name in provider broker untrusted; do
      openssl req -x509 -newkey rsa:2048 -nodes -days 36500 \
        -keyout "$out/$name-ca.key" -out "$out/$name-ca.pem" \
        -subj "/CN=Den test $name CA" -addext basicConstraints=critical,CA:TRUE
      openssl req -new -newkey rsa:2048 -nodes \
        -keyout "$out/$name.key" -out "$out/$name.csr" -subj /CN=localhost
      printf 'subjectAltName=DNS:localhost\nextendedKeyUsage=serverAuth\n' > extensions
      openssl x509 -req -days 36500 -in "$out/$name.csr" \
        -CA "$out/$name-ca.pem" -CAkey "$out/$name-ca.key" -CAcreateserial \
        -extfile extensions -out "$out/$name.pem"
    done
    cat ${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt "$out/provider-ca.pem" \
      > "$out/etc/ssl/certs/ca-bundle.crt"
  '';
  # Replace only the package-set CA input, not production code or runtime trust.
  # The generated adapter still invokes the real packaged Pi and dispatcher.
  adapter = (import ../lib/mk-pi.nix {
    inherit inputs;
    pkgs = pkgs // { cacert = certificates; };
    mkAgentSandbox = value: value;
    isDarwin = false;
  }) { };
  extension = pkgs.writeText "den-pi-tls-test.ts" ''
    export default function (pi: any) {
      pi.registerCommand("tls-test", { description: "Offline TLS regression", handler: async () => {
        const { checkTrust } = await import(${builtins.toJSON "${./fixtures/pi/tls.test.mjs}"});
        await checkTrust();
      }});
    }
  '';
in
pkgs.runCommand "pi-tls" {
  nativeBuildInputs = [ pkgs.nodejs_22 ];
  __darwinAllowLocalNetworking = true;
} ''
  export PI_TLS_FIXTURES=${certificates}
  export PI_TLS_AGENT=${adapter.adapter.agent.executable}
  export PI_TLS_EXTENSION=${extension}
  export PI_PACKAGE_DIR=${adapter.adapter.agent.packageDirectory.value}
  export REPOWOLF_CA_FILE=${certificates}/broker-ca.pem
  export REPOWOLF_CLIENT_DIR="$TMPDIR/unused-client"
  export PI_CODING_AGENT_DIR="$TMPDIR/agent"
  export PI_CODING_AGENT_SESSION_DIR="$TMPDIR/sessions"
  export PI_OFFLINE=1
  export SSL_CERT_FILE="$TMPDIR/missing-host-ca.pem"
  export NIX_SSL_CERT_FILE=${certificates}/untrusted-ca.pem
  export NODE_EXTRA_CA_CERTS=${certificates}/untrusted-ca.pem
  mkdir -p "$PI_CODING_AGENT_DIR" "$PI_CODING_AGENT_SESSION_DIR"
  node ${./fixtures/pi/tls.test.mjs}
  touch "$out"
''
