//go:build native

package pi

import "testing"

func TestPiPublicCATrustIsReadableInsideFence(t *testing.T) {
	fixture := newPiFixture(t)
	fixture.enforcementProbe(t, `
 const { X509Certificate } = await import("node:crypto");
 const file = process.env.SSL_CERT_FILE!;
 const pem = fs.readFileSync(file, "utf8");
 assert.match(file, /^\/nix\/store\//, "public CA trust must not depend on host /etc");
 const roots = pem.match(/-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----/g) ?? [];
 assert.ok(roots.length > 0, "public CA bundle must contain certificates");
 assert.equal(process.env.NIX_SSL_CERT_FILE, file, "Nix OpenSSL trust must match Node trust");
 // Node's "system" CA enumeration uses macOS Keychain, not SSL_CERT_FILE.
 for (const root of roots) assert.ok(new X509Certificate(root).ca, "Node must parse valid public CA roots inside Fence");
 assert.throws(() => fs.appendFileSync(file, "mutation"), "public CA bundle must be read-only");
 assert.equal(process.env.NODE_EXTRA_CA_CERTS, process.env.REPOWOLF_CA_FILE, "broker CA must stay separate");
 record("public-ca-readable-and-immutable");
 `, []string{"SSL_CERT_FILE=/host-trust-unavailable.pem", "NIX_SSL_CERT_FILE=/host-nix-trust-unavailable.pem"})
}
