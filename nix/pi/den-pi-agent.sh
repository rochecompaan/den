# Nix Node uses OpenSSL's CA store; the host /etc bundle is hidden by Fence.
export SSL_CERT_FILE=@publicCA@
# Nix OpenSSL gives this variable precedence over SSL_CERT_FILE.
export NIX_SSL_CERT_FILE="$SSL_CERT_FILE"
export NODE_EXTRA_CA_CERTS="$REPOWOLF_CA_FILE"
export GIT_SSH_COMMAND="$REPOWOLF_CLIENT_DIR/bin/repowolf-git-ssh"
exec @pi@/bin/pi "$@"
