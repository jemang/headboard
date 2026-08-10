package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jemang/headboard/internal/hs"
)

func TestUpstreamPreservesHeadscaleClientError(t *testing.T) {
	err := upstream(&hs.APIError{
		Status: http.StatusBadRequest,
		Body:   `{"message":"name must not contain a period"}`,
	}, "could not rename the device")

	status, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("upstream error %T does not carry an HTTP status", err)
	}
	if status.GetStatus() != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status.GetStatus(), http.StatusBadRequest)
	}
	if status.Error() != "name must not contain a period" {
		t.Errorf("detail = %q, want Headscale validation reason", status.Error())
	}
}

func TestUpstreamKeepsUnavailableHeadscaleAsGatewayError(t *testing.T) {
	err := upstream(errors.New("connection refused"), "could not set tags")

	status, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("upstream error %T does not carry an HTTP status", err)
	}
	if status.GetStatus() != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", status.GetStatus(), http.StatusBadGateway)
	}
	if status.Error() != "could not set tags" {
		t.Errorf("detail = %q, want safe fallback", status.Error())
	}
}

func TestUpstreamDoesNotExposeUnstructuredClientErrorBodies(t *testing.T) {
	err := upstream(&hs.APIError{
		Status: http.StatusBadRequest,
		Body:   "proxy validation failed at http://headscale.internal/api/v1/node",
	}, "could not rename the device")

	status, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("upstream error %T does not carry an HTTP status", err)
	}
	if status.GetStatus() != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", status.GetStatus(), http.StatusBadRequest)
	}
	if status.Error() != "could not rename the device" {
		t.Errorf("detail = %q, want safe fallback", status.Error())
	}
}

func TestPolicyCheckDetailPreservesHeadscaleValidationMessage(t *testing.T) {
	detail := policyCheckDetail(&hs.APIError{
		Status: http.StatusBadRequest,
		Body:   `{"code":3,"message":"group not defined in policy: \"group:ops\""}`,
	})

	if detail != `group not defined in policy: "group:ops"` {
		t.Fatalf("detail = %q, want Headscale validation message", detail)
	}
}

func TestValidateDeviceNameRejectsPeriodsBeforeHeadscale(t *testing.T) {
	err := validateDeviceName("alices.laptop")

	status, ok := err.(huma.StatusError)
	if !ok {
		t.Fatalf("validation error %T does not carry an HTTP status", err)
	}
	if status.GetStatus() != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d", status.GetStatus(), http.StatusUnprocessableEntity)
	}
	if status.Error() != "device names cannot contain a period" {
		t.Errorf("detail = %q, want actionable validation message", status.Error())
	}
}
