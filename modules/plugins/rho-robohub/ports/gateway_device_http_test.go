package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A device that answers with a redirect does not send the gateway elsewhere: the
// redirect is not followed, and neither it nor an error page reads as a status.
func TestTheGatewayDoesNotFollowADevicesRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.Write([]byte(`{"output":true}`))
	}))
	defer target.Close()
	device := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, target.URL+"/", http.StatusFound)
		case "/error":
			http.Error(w, "internal detail", http.StatusInternalServerError)
		default:
			w.Write([]byte(`{"output":false}`))
		}
	}))
	defer device.Close()

	if got := gatewayHttpGet(device.URL+"/redirect", 2000); got != "" || reached {
		t.Fatalf("redirect followed: got %q, reached %v", got, reached)
	}
	if got := gatewayHttpGet(device.URL+"/error", 2000); got != "" {
		t.Fatalf("an error page read as a status: %q", got)
	}
	if got := gatewayHttpGet(device.URL+"/", 2000); got != `{"output":false}` {
		t.Fatalf("a device's own answer was lost: %q", got)
	}
	if gatewayHttpPost(device.URL+"/redirect", "{}", 2000) || reached {
		t.Fatalf("a redirected POST counted as done, or was followed")
	}
}
