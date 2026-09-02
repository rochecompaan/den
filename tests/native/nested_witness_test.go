package native

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNestedFenceWitness(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(proxy.Close)
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		outerParent    int
		outerProxy     string
		innerProxy     string
		outerSandbox   string
		disableErrexit bool
		wantSuccess    bool
	}{
		{name: "nested context", outerParent: 1, outerProxy: "http://127.0.0.1:1", innerProxy: proxy.URL, wantSuccess: true},
		{name: "shared parent and proxy", outerParent: os.Getpid(), outerProxy: proxy.URL, innerProxy: proxy.URL, wantSuccess: true},
		{name: "shared active proxy", outerParent: 1, outerProxy: proxy.URL, innerProxy: proxy.URL, wantSuccess: true},
		{name: "equal parent with nested proxy", outerParent: os.Getpid(), outerProxy: "http://127.0.0.1:1", innerProxy: proxy.URL, wantSuccess: true},
		{name: "inactive nested proxy", outerParent: 1, outerProxy: "http://127.0.0.1:1", innerProxy: "http://127.0.0.1:2", disableErrexit: true},
		{name: "invalid outer sandbox without errexit", outerParent: 1, outerProxy: "http://127.0.0.1:1", innerProxy: proxy.URL, outerSandbox: "0", disableErrexit: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			record := filepath.Join(directory, ".den-native-outer-context")
			outerSandbox := test.outerSandbox
			if outerSandbox == "" {
				outerSandbox = "1"
			}
			contents := fmt.Sprintf("%d\n%s\n%s\n", test.outerParent, test.outerProxy, outerSandbox)
			if err := os.WriteFile(record, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			script := nestedFenceWitnessCommand()
			if test.disableErrexit {
				script = strings.Replace(script, "\n", "\nset +e\n", 1)
			}
			command := exec.Command("sh", "-c", script)
			command.Env = []string{
				"DEN_FENCE_TMPDIR=" + directory,
				"DEN_NATIVE_CONTEXT_CURL=" + curl,
				"FENCE_SANDBOX=1",
				"HTTP_PROXY=" + test.innerProxy,
			}
			err := command.Run()
			if (err == nil) != test.wantSuccess {
				t.Fatalf("nested witness success = %v, want %v", err == nil, test.wantSuccess)
			}
		})
	}
}
