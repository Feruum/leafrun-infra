package main

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Keep provider payloads, URLs, request IDs and credentials out of startup logs.
func readinessDiagnostic(err error) string {
	code, status := "other", 0
	var api smithy.APIError
	if errors.As(err, &api) {
		switch api.ErrorCode() {
		case "NotFound", "NoSuchBucket", "Forbidden", "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch", "ServiceUnavailable", "InternalError", "UnknownError", "RequestTimeout":
			code = api.ErrorCode()
		}
	}
	var response *smithyhttp.ResponseError
	if errors.As(err, &response) {
		status = response.HTTPStatusCode()
	}
	var operation *net.OpError
	transport := errors.As(err, &operation)
	var network net.Error
	timeout := errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout())
	return fmt.Sprintf("api=%s status=%d transport=%t timeout=%t", code, status, transport, timeout)
}
