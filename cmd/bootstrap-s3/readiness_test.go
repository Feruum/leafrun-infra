package main

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestReadinessDiagnosticExcludesProviderPayloadAndCredentials(t *testing.T) {
	err := &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: 403}}, Err: &smithy.GenericAPIError{Code: "AccessDenied", Message: "private-provider-payload"}}
	diagnostic := readinessDiagnostic(err)
	if !strings.Contains(diagnostic, "api=AccessDenied") || !strings.Contains(diagnostic, "status=403") || strings.Contains(diagnostic, "private-provider-payload") {
		t.Fatal("safe provider diagnostic changed")
	}
	transport := &url.Error{Op: "Head", URL: "http://private.invalid/?token=private-token", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("private-transport-detail")}}
	diagnostic = readinessDiagnostic(transport)
	if !strings.Contains(diagnostic, "transport=true") || strings.Contains(diagnostic, "private") {
		t.Fatal("transport diagnostic leaked request details")
	}
	diagnostic = readinessDiagnostic(&smithy.GenericAPIError{Code: "private-access-key", Message: "private-provider-payload"})
	if strings.Contains(diagnostic, "private") || !strings.Contains(diagnostic, "api=other") {
		t.Fatal("unknown provider code escaped the allowlist")
	}
}
