export NODE_EXTRA_CA_CERTS="$REPOWOLF_CA_FILE"
export GIT_SSH_COMMAND="$REPOWOLF_CLIENT_DIR/bin/repowolf-git-ssh"
exec @pi@/bin/pi "$@"
