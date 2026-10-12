package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jemang/headboard/internal/sshsync"
)

func TestClientSyncHandlesETagAndAuth(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer hbk_ok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("If-None-Match") == `"1.1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"1.1"`)
		json.NewEncoder(w).Encode(sshsync.SyncResponse{Rev: "1.1", Protocol: sshsync.Protocol, Targets: []sshsync.Target{{Ref: "105"}}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "hbk_ok", srv.Client())

	resp, tag, changed, err := c.Sync(t.Context(), "")
	if err != nil || !changed || tag != `"1.1"` || len(resp.Targets) != 1 {
		t.Fatalf("Sync = %+v %q %v %v", resp, tag, changed, err)
	}

	_, tag2, changed, err := c.Sync(t.Context(), tag)
	if err != nil || changed || tag2 != tag {
		t.Fatalf("304 Sync = %q %v %v", tag2, changed, err)
	}

	bad := NewClient(srv.URL, "hbk_bad", srv.Client())
	if _, _, _, err := bad.Sync(t.Context(), ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("bad token err = %v", err)
	}
}

func TestClientRefusesProtocolMismatch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(sshsync.SyncResponse{Protocol: sshsync.Protocol + 1})
	}))
	defer srv.Close()

	if _, _, _, err := NewClient(srv.URL, "hbk_ok", srv.Client()).Sync(t.Context(), ""); err == nil {
		t.Fatal("accepted a newer protocol")
	}
}

func TestClientRefusesRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(sshsync.SyncResponse{Protocol: sshsync.Protocol})
	}))
	defer plain.Close()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/api/ssh/agent/sync", http.StatusFound)
	}))
	defer srv.Close()

	if _, _, _, err := NewClient(srv.URL, "hbk_ok", srv.Client()).Sync(t.Context(), ""); err == nil {
		t.Fatal("followed a redirect to http")
	}
}

func TestClientRefusesAnOversizedSyncBody(t *testing.T) {
	saved := maxJSON
	maxJSON = 64
	t.Cleanup(func() { maxJSON = saved })

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"rev":"1","protocol":%d,"targets":[],"pad":"%s"}`, sshsync.Protocol, strings.Repeat("x", 500))
	}))
	defer srv.Close()

	if _, _, _, err := NewClient(srv.URL, "hbk_x", srv.Client()).Sync(t.Context(), ""); err == nil {
		t.Fatal("an oversized sync body was accepted")
	}
}
