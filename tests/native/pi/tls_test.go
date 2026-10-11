//go:build native

package pi

import "testing"

func TestPiPublicCATrustIsReadableInsideFence(t *testing.T) {
	fixture := newPiFixture(t)
	fixture.enforcementProbe(t, `
 const tls = await import("node:tls");
 const file = process.env.SSL_CERT_FILE!;
 const pem = fs.readFileSync(file, "utf8");
 assert.match(file, /^\/nix\/store\//, "public CA trust must not depend on host /etc");
 assert.match(pem, /-----BEGIN CERTIFICATE-----/);
 assert.equal(process.env.NIX_SSL_CERT_FILE, file, "Nix OpenSSL trust must match Node trust");
 assert.ok(tls.getCACertificates("system").length > 0, "Node must load public roots inside Fence");
 assert.throws(() => fs.appendFileSync(file, "mutation"), "public CA bundle must be read-only");
 assert.equal(process.env.NODE_EXTRA_CA_CERTS, process.env.REPOWOLF_CA_FILE, "broker CA must stay separate");
 record("public-ca-readable-and-immutable");
 `, []string{"SSL_CERT_FILE=/host-trust-unavailable.pem", "NIX_SSL_CERT_FILE=/host-nix-trust-unavailable.pem"})
}
