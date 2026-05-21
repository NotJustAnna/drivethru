package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dtv1alpha1 "github.com/notjustanna/drivethru/api/v1alpha1"
	"github.com/notjustanna/drivethru/internal/config"
	"github.com/notjustanna/drivethru/internal/garage"
	"github.com/notjustanna/drivethru/internal/traefik"
)

// fakeGarage is an in-memory stand-in for garage.Client.
type fakeGarage struct {
	buckets map[string]string // host -> bucketID
	keys    map[string]string // name -> accessKeyID
	secrets map[string]string // accessKeyID -> secret
	allowed map[string]bool   // "bid|akid" -> true
	denied  map[string]bool

	createBucketCount int
	createKeyCount    int

	failEnsureBucket error
}

func newFakeGarage() *fakeGarage {
	return &fakeGarage{
		buckets: map[string]string{},
		keys:    map[string]string{},
		secrets: map[string]string{},
		allowed: map[string]bool{},
		denied:  map[string]bool{},
	}
}

func (f *fakeGarage) EnsureBucket(_ context.Context, host string) (string, error) {
	if f.failEnsureBucket != nil {
		return "", f.failEnsureBucket
	}
	if id, ok := f.buckets[host]; ok {
		return id, nil
	}
	f.createBucketCount++
	id := "bid-" + host
	f.buckets[host] = id
	return id, nil
}

func (f *fakeGarage) LookupBucket(_ context.Context, host string) (string, bool, error) {
	id, ok := f.buckets[host]
	return id, ok, nil
}

func (f *fakeGarage) EnableWebsite(_ context.Context, _ string) error { return nil }

func (f *fakeGarage) EnsureKey(_ context.Context, name string) (garage.KeyInfo, bool, error) {
	if id, ok := f.keys[name]; ok {
		return garage.KeyInfo{AccessKeyID: id, Name: name}, false, nil
	}
	f.createKeyCount++
	id := "GK-" + name
	secret := "sec-" + name
	f.keys[name] = id
	f.secrets[id] = secret
	return garage.KeyInfo{AccessKeyID: id, Name: name, SecretAccessKey: secret}, true, nil
}

func (f *fakeGarage) LookupKey(_ context.Context, name string) (string, bool, error) {
	id, ok := f.keys[name]
	return id, ok, nil
}

func (f *fakeGarage) AllowKeyOnBucket(_ context.Context, bid, akid string) error {
	f.allowed[bid+"|"+akid] = true
	return nil
}

func (f *fakeGarage) DenyKeyOnBucket(_ context.Context, bid, akid string) error {
	f.denied[bid+"|"+akid] = true
	delete(f.allowed, bid+"|"+akid)
	return nil
}

func (f *fakeGarage) DeleteBucket(_ context.Context, bid string) error {
	for h, id := range f.buckets {
		if id == bid {
			delete(f.buckets, h)
		}
	}
	return nil
}

func (f *fakeGarage) DeleteKey(_ context.Context, akid string) error {
	for n, id := range f.keys {
		if id == akid {
			delete(f.keys, n)
		}
	}
	delete(f.secrets, akid)
	return nil
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := dtv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := traefik.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func newReconciler(t *testing.T, objs ...client.Object) (*StaticSiteReconciler, *fakeGarage) {
	t.Helper()
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&dtv1alpha1.StaticSite{}).
		Build()
	fg := newFakeGarage()
	r := &StaticSiteReconciler{
		Client: c,
		Scheme: scheme,
		Cfg: &config.Config{
			GarageS3Endpoint:       "http://garage.svc:3900",
			GarageAdminEndpoint:    "http://garage.svc:3903",
			GarageAdminToken:       "tok",
			TraefikEntrypoint:      "websecure",
			TraefikCertResolver:    "letsencrypt",
			GarageServiceName:      "garage",
			GarageServiceNamespace: "garage-system",
			GarageServicePort:      3902,
		},
		Garage: fg,
	}
	return r, fg
}

func newSite(name, host string, mutate func(*dtv1alpha1.StaticSite)) *dtv1alpha1.StaticSite {
	s := &dtv1alpha1.StaticSite{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "my-app",
		},
		Spec: dtv1alpha1.StaticSiteSpec{Host: host},
	}
	if mutate != nil {
		mutate(s)
	}
	return s
}

func reconcile(t *testing.T, r *StaticSiteReconciler, name string) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: "my-app"},
	}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
}

func TestReconcile_CreatesEverything(t *testing.T) {
	site := newSite("my-site", "my-site.example", nil)
	r, fg := newReconciler(t, site)

	// First reconcile adds finalizer.
	reconcile(t, r, "my-site")
	// Second reconcile does the actual work.
	reconcile(t, r, "my-site")

	if fg.createBucketCount != 1 || fg.createKeyCount != 1 {
		t.Errorf("create counts: bucket=%d key=%d", fg.createBucketCount, fg.createKeyCount)
	}

	var secret corev1.Secret
	if err := r.Get(context.Background(), types.NamespacedName{
		Name: "my-site-s3", Namespace: "my-app"}, &secret); err != nil {
		t.Fatalf("secret missing: %v", err)
	}
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "BUCKET_NAME", "AWS_DEFAULT_REGION", "AWS_ENDPOINT_URL"} {
		if _, ok := secret.Data[k]; !ok {
			t.Errorf("secret missing key %s", k)
		}
	}
	if string(secret.Data["BUCKET_NAME"]) != "my-site.example" {
		t.Errorf("bucket name: %q", secret.Data["BUCKET_NAME"])
	}

	var ir traefik.IngressRoute
	if err := r.Get(context.Background(), types.NamespacedName{
		Name: "my-site", Namespace: "my-app"}, &ir); err != nil {
		t.Fatalf("ingressroute missing: %v", err)
	}
	if got := ir.Spec.Routes[0].Match; got != "Host(`my-site.example`)" {
		t.Errorf("match: %q", got)
	}
	if ir.Spec.TLS.CertResolver != "letsencrypt" {
		t.Errorf("certResolver: %q", ir.Spec.TLS.CertResolver)
	}

	var got dtv1alpha1.StaticSite
	if err := r.Get(context.Background(), types.NamespacedName{Name: "my-site", Namespace: "my-app"}, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Status.Ready {
		t.Error("status.ready should be true")
	}
}

func TestReconcile_Idempotent(t *testing.T) {
	site := newSite("my-site", "my-site.example", nil)
	r, fg := newReconciler(t, site)

	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")

	if fg.createBucketCount != 1 {
		t.Errorf("bucket created %d times", fg.createBucketCount)
	}
	if fg.createKeyCount != 1 {
		t.Errorf("key created %d times", fg.createKeyCount)
	}
}

func TestReconcile_SecretDisabled(t *testing.T) {
	site := newSite("my-site", "my-site.example", func(s *dtv1alpha1.StaticSite) {
		s.Spec.GeneratedSecretName = &dtv1alpha1.SecretNameOption{Disabled: true}
	})
	r, fg := newReconciler(t, site)
	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")

	var secret corev1.Secret
	err := r.Get(context.Background(), types.NamespacedName{Name: "my-site-s3", Namespace: "my-app"}, &secret)
	if !apierrors.IsNotFound(err) {
		t.Errorf("expected no secret, got err=%v", err)
	}
	if fg.createKeyCount != 0 {
		t.Errorf("expected no key created, got %d", fg.createKeyCount)
	}
}

func TestReconcile_CustomSecretName(t *testing.T) {
	site := newSite("my-site", "my-site.example", func(s *dtv1alpha1.StaticSite) {
		s.Spec.GeneratedSecretName = &dtv1alpha1.SecretNameOption{Name: "custom-creds"}
	})
	r, _ := newReconciler(t, site)
	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")

	var secret corev1.Secret
	if err := r.Get(context.Background(), types.NamespacedName{Name: "custom-creds", Namespace: "my-app"}, &secret); err != nil {
		t.Errorf("custom-named secret missing: %v", err)
	}
}

func TestReconcile_DeleteCleansUp(t *testing.T) {
	site := newSite("my-site", "my-site.example", nil)
	r, fg := newReconciler(t, site)
	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")

	// Delete triggers DeletionTimestamp; finalizer keeps the object around.
	var fresh dtv1alpha1.StaticSite
	if err := r.Get(context.Background(), types.NamespacedName{Name: "my-site", Namespace: "my-app"}, &fresh); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(context.Background(), &fresh); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "my-site")

	if len(fg.buckets) != 0 {
		t.Errorf("bucket not deleted: %+v", fg.buckets)
	}
	if len(fg.keys) != 0 {
		t.Errorf("key not deleted: %+v", fg.keys)
	}
}

func TestReconcile_RetainSkipsGarageCleanup(t *testing.T) {
	site := newSite("my-site", "my-site.example", func(s *dtv1alpha1.StaticSite) {
		s.Spec.Retain = true
	})
	r, fg := newReconciler(t, site)
	reconcile(t, r, "my-site")
	reconcile(t, r, "my-site")

	var fresh dtv1alpha1.StaticSite
	if err := r.Get(context.Background(), types.NamespacedName{Name: "my-site", Namespace: "my-app"}, &fresh); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(context.Background(), &fresh); err != nil {
		t.Fatal(err)
	}
	reconcile(t, r, "my-site")

	if len(fg.buckets) != 1 {
		t.Errorf("bucket should still exist with retain=true: %+v", fg.buckets)
	}
	if len(fg.keys) != 1 {
		t.Errorf("key should still exist with retain=true: %+v", fg.keys)
	}
}

func TestReconcile_BucketFailureMarksNotReady(t *testing.T) {
	site := newSite("my-site", "my-site.example", nil)
	r, fg := newReconciler(t, site)
	fg.failEnsureBucket = errors.New("garage offline")

	reconcile(t, r, "my-site") // add finalizer
	if _, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "my-site", Namespace: "my-app"},
	}); err == nil {
		t.Fatal("expected error")
	}

	var got dtv1alpha1.StaticSite
	if err := r.Get(context.Background(), types.NamespacedName{Name: "my-site", Namespace: "my-app"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Ready {
		t.Error("status.ready should be false on failure")
	}
}

func TestSecretNameOption_UnmarshalJSON(t *testing.T) {
	cases := []struct {
		in       string
		disabled bool
		name     string
		wantErr  bool
	}{
		{`"my-name"`, false, "my-name", false},
		{`false`, true, "", false},
		{`null`, false, "", false},
		{`true`, false, "", true},
		{`42`, false, "", true},
	}
	for _, tc := range cases {
		var s dtv1alpha1.SecretNameOption
		err := s.UnmarshalJSON([]byte(tc.in))
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if tc.wantErr {
			continue
		}
		if s.Disabled != tc.disabled || s.Name != tc.name {
			t.Errorf("%s: got %+v", tc.in, s)
		}
	}
}
