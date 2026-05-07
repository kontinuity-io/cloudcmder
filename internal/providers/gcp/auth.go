// Package gcp implements the inventory.Provider interface against Google Cloud.
package gcp

import (
	"context"

	"golang.org/x/oauth2/google"
)

// readOnlyScope is the OAuth scope requested for ADC. On CloudShell the
// session token's actual scopes win — the requested scope here is advisory
// for non-CloudShell paths (laptop `gcloud auth application-default login`
// and SA-bound GCE VMs).
const readOnlyScope = "https://www.googleapis.com/auth/cloud-platform.read-only"

// NewCredentials resolves Application Default Credentials.
func NewCredentials(ctx context.Context) (*google.Credentials, error) {
	return google.FindDefaultCredentials(ctx, readOnlyScope)
}
