package gcp

import (
	"errors"
	"testing"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsRecoverableScanErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "plain error", err: errors.New("network down"), want: false},
		{name: "grpc PermissionDenied", err: status.Error(codes.PermissionDenied, "API disabled"), want: true},
		{name: "grpc Unauthenticated", err: status.Error(codes.Unauthenticated, "no creds"), want: false},
		{name: "grpc NotFound", err: status.Error(codes.NotFound, "no such project"), want: false},
		{name: "googleapi 403", err: &googleapi.Error{Code: 403, Message: "API disabled"}, want: true},
		{name: "googleapi 404", err: &googleapi.Error{Code: 404}, want: false},
		{name: "wrapped googleapi 403", err: errors.New("wrapper: " + (&googleapi.Error{Code: 403, Message: "x"}).Error()), want: false}, // string wrapping doesn't preserve type
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRecoverableScanErr(tc.err); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
