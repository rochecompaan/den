package repowolf

import "testing"

func TestLoadEnvAcceptsIPLiteralEndpoints(t *testing.T) {
	for _, test := range []struct {
		endpoint string
		host     string
	}{
		{"https://172.17.0.1:8443", "172.17.0.1"},
		{"https://172.17.0.1:8443/", "172.17.0.1"},
		{"https://172.17.0.1", "172.17.0.1"},
		{"https://127.0.0.1/", "127.0.0.1"},
		{"https://192.0.2.1:443", "192.0.2.1"},
		{"https://[::1]", "::1"},
		{"https://[::1]/", "::1"},
		{"https://[2001:db8::1]:8443/", "2001:db8::1"},
		{"https://[2001:db8::1]", "2001:db8::1"},
		{"https://[2001:0DB8:0:0:0:0:0:1]:8443", "2001:0DB8:0:0:0:0:0:1"},
		{"https://[::ffff:192.0.2.1]:8443", "::ffff:192.0.2.1"},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			values := validValues()
			values["REPOWOLF_ENDPOINT"] = test.endpoint
			config, err := LoadEnv(lookup(values), readableCA)
			if err != nil {
				t.Fatalf("LoadEnv() error = %v", err)
			}
			if config.Endpoint != test.endpoint || config.Hostname != test.host {
				t.Fatalf("endpoint = %q, host = %q; want %q, %q", config.Endpoint, config.Hostname, test.endpoint, test.host)
			}
		})
	}
}

func TestLoadEnvRejectsInvalidIPLiteralEndpoints(t *testing.T) {
	for _, endpoint := range []string{
		"http://172.17.0.1:8443",
		"http://[::1]:8443",
		"https://user@172.17.0.1:8443",
		"https://user@[::1]:8443",
		"https://172.17.0.1/non-root",
		"https://[::1]/non-root",
		"https://172.17.0.1/?query=yes",
		"https://[::1]/?",
		"https://172.17.0.1/#fragment",
		"https://[::1]/#",
		"https://172.17.0.1:",
		"https://[::1]:",
		"https://172.17.0.1:0",
		"https://[::1]:0",
		"https://172.17.0.1:08443",
		"https://[::1]:08443",
		"https://172.17.0.1:65536",
		"https://[::1]:65536",
		"https://2001:db8::1",
		"https://2001:db8::1:8443",
		"https://[172.17.0.1]:8443",
		"https://[2001:db8:::1]:8443",
		"https://[fe80::1%25eth0]:8443",
	} {
		t.Run(endpoint, func(t *testing.T) {
			values := validValues()
			values["REPOWOLF_ENDPOINT"] = endpoint
			_, err := LoadEnv(lookup(values), readableCA)
			assertRedacted(t, err, "REPOWOLF_ENDPOINT", endpoint, testToken, testCA)
		})
	}
}
