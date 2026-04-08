package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPageserverHTTPAttachmentClientGetLSNByTimestampUsesExpectedPathAndQuery(t *testing.T) {
	restoreAt := time.Date(2010, 1, 2, 3, 4, 5, 0, time.UTC)

	var gotPath string
	var gotTimestamp string
	var gotWithLease string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTimestamp = r.URL.Query().Get("timestamp")
		gotWithLease = r.URL.Query().Get("with_lease")
		_, _ = w.Write([]byte(`{"kind":"future","lsn":"0/16B6F50"}`))
	}))
	defer ts.Close()

	client, err := newPageserverHTTPAttachmentClient(ts.URL, 16, ts.Client())
	if err != nil {
		t.Fatalf("new pageserver client: %v", err)
	}

	kind, lsn, err := client.GetLSNByTimestamp("tenant-a", "timeline-a", restoreAt)
	if err != nil {
		t.Fatalf("get lsn by timestamp: %v", err)
	}

	if kind != "future" {
		t.Fatalf("expected kind %q, got %q", "future", kind)
	}

	if lsn != "0/16B6F50" {
		t.Fatalf("expected lsn %q, got %q", "0/16B6F50", lsn)
	}

	if gotPath != "/v1/tenant/tenant-a/timeline/timeline-a/get_lsn_by_timestamp" {
		t.Fatalf("expected request path %q, got %q", "/v1/tenant/tenant-a/timeline/timeline-a/get_lsn_by_timestamp", gotPath)
	}

	if gotTimestamp != restoreAt.Format(time.RFC3339) {
		t.Fatalf("expected timestamp query %q, got %q", restoreAt.Format(time.RFC3339), gotTimestamp)
	}

	if gotWithLease != "false" {
		t.Fatalf("expected with_lease query %q, got %q", "false", gotWithLease)
	}
}
