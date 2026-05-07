package gcp

import (
	"errors"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsRecoverableScanErr reports whether err from an enrichment step represents
// a per-kind permission or API-disabled failure that should NOT abort the
// whole scan. Architecture.md §"Error Handling" mandates this: skip the kind,
// keep whatever was already emitted, surface the issue as a warning.
//
// Covers both the gRPC SDK shape (run/apiv2, functions/apiv2, container,
// asset, instances) and the REST autogen shape (sqladmin, storage). Both
// surface SERVICE_DISABLED as 403 / PermissionDenied so a single check
// does the job.
func IsRecoverableScanErr(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.PermissionDenied {
		return true
	}
	var ge *googleapi.Error
	if errors.As(err, &ge) && ge.Code == 403 {
		return true
	}
	return false
}
