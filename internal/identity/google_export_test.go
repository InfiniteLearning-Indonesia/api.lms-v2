//go:build password_rehearsal

package identity

import "net/http"

// SetGoogleFixtureTransport allows the external PostgreSQL test to impersonate
// the provider at the transport boundary. It is excluded from runtime builds.
func SetGoogleFixtureTransport(service *Google, transport http.RoundTripper) {
	service.provider.client.Transport = transport
}
