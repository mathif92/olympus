package pkg

import (
	"errors"
	"fmt"
)

// renderKubeconfig builds a single-context kubeconfig document for a managed
// cluster. caData must be the base64-encoded PEM of the API-server CA.
func renderKubeconfig(name, server, caData string) (string, error) {
	if name == "" || server == "" {
		return "", errors.New("cluster name and endpoint are required")
	}
	if caData == "" {
		return "", errors.New("missing API-server CA data")
	}
	return fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %s
  cluster:
    server: %s
    certificate-authority-data: %s
contexts:
- name: %s
  context:
    cluster: %s
    user: %s
current-context: %s
users:
- name: %s
  user:
    token: placeholder-token
`, name, server, caData, name, name, name, name, name), nil
}
