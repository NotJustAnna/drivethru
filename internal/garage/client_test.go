package garage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordedCall struct {
	method string
	path   string
	query  string
	body   string
}

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *[]recordedCall) {
	t.Helper()
	calls := &[]recordedCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*calls = append(*calls, recordedCall{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			body:   string(b),
		})
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("auth header: got %q", got)
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c := Client(New(srv.URL, "test-token"))
	return &c, calls
}

func TestEnsureBucket_AlreadyExists(t *testing.T) {
	c, calls := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/GetBucketInfo" {
			_ = json.NewEncoder(w).Encode(BucketInfo{ID: "abc"})
			return
		}
		t.Fatalf("unexpected call: %s %s", r.Method, r.URL.Path)
	})
	id, err := (*c).EnsureBucket(context.Background(), "site.example")
	if err != nil {
		t.Fatal(err)
	}
	if id != "abc" {
		t.Errorf("id: got %q", id)
	}
	if len(*calls) != 1 {
		t.Errorf("expected 1 call, got %d", len(*calls))
	}
}

func TestEnsureBucket_Creates(t *testing.T) {
	c, calls := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/GetBucketInfo":
			w.WriteHeader(http.StatusNotFound)
		case "/v2/CreateBucket":
			_ = json.NewEncoder(w).Encode(BucketInfo{ID: "new-id"})
		default:
			t.Fatalf("unexpected call: %s", r.URL.Path)
		}
	})
	id, err := (*c).EnsureBucket(context.Background(), "site.example")
	if err != nil {
		t.Fatal(err)
	}
	if id != "new-id" {
		t.Errorf("id: %q", id)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(*calls))
	}
	if !strings.Contains((*calls)[1].body, `"globalAlias":"site.example"`) {
		t.Errorf("CreateBucket body wrong: %s", (*calls)[1].body)
	}
}

func TestEnableWebsite(t *testing.T) {
	c, calls := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := (*c).EnableWebsite(context.Background(), "bid"); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].path != "/v2/UpdateBucket" || (*calls)[0].query != "id=bid" {
		t.Errorf("wrong call: %+v", (*calls)[0])
	}
	if !strings.Contains((*calls)[0].body, `"enabled":true`) ||
		!strings.Contains((*calls)[0].body, `"indexDocument":"index.html"`) {
		t.Errorf("websiteAccess body wrong: %s", (*calls)[0].body)
	}
}

func TestEnsureKey_Reuses(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/GetKeyInfo" {
			_ = json.NewEncoder(w).Encode(KeyInfo{AccessKeyID: "GK1", Name: "drivethru-foo"})
			return
		}
		t.Fatalf("unexpected: %s", r.URL.Path)
	})
	info, created, err := (*c).EnsureKey(context.Background(), "drivethru-foo")
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("should not have created")
	}
	if info.AccessKeyID != "GK1" {
		t.Errorf("access key: %q", info.AccessKeyID)
	}
}

func TestEnsureKey_Creates(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/GetKeyInfo":
			w.WriteHeader(http.StatusNotFound)
		case "/v2/CreateKey":
			_ = json.NewEncoder(w).Encode(KeyInfo{AccessKeyID: "GK2", SecretAccessKey: "s2"})
		default:
			t.Fatalf("unexpected: %s", r.URL.Path)
		}
	})
	info, created, err := (*c).EnsureKey(context.Background(), "drivethru-bar")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("should have created")
	}
	if info.SecretAccessKey != "s2" {
		t.Errorf("secret: %q", info.SecretAccessKey)
	}
}

func TestAllowKeyOnBucket(t *testing.T) {
	c, calls := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := (*c).AllowKeyOnBucket(context.Background(), "bid", "akid"); err != nil {
		t.Fatal(err)
	}
	body := (*calls)[0].body
	if !strings.Contains(body, `"bucketId":"bid"`) || !strings.Contains(body, `"accessKeyId":"akid"`) {
		t.Errorf("body: %s", body)
	}
	if !strings.Contains(body, `"read":true`) || !strings.Contains(body, `"write":true`) {
		t.Errorf("permissions: %s", body)
	}
}

func TestDeleteBucket_NotFoundIsOK(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if err := (*c).DeleteBucket(context.Background(), "missing"); err != nil {
		t.Errorf("expected nil on 404, got %v", err)
	}
}

func TestServerError_Surfaces(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	})
	_, err := (*c).EnsureBucket(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got %v", err)
	}
}

func TestLookupBucket_NotFound(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	_, found, err := (*c).LookupBucket(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("should not be found")
	}
}
