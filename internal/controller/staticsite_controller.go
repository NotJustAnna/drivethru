// Package controller hosts the StaticSite reconciler.
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dtv1alpha1 "github.com/notjustanna/drivethru/api/v1alpha1"
	"github.com/notjustanna/drivethru/internal/config"
	"github.com/notjustanna/drivethru/internal/garage"
	"github.com/notjustanna/drivethru/internal/traefik"
)

// StaticSiteReconciler reconciles a StaticSite resource.
type StaticSiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cfg    *config.Config
	Garage garage.Client
}

// +kubebuilder:rbac:groups=drivethru.notjustanna.net,resources=staticsites,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=drivethru.notjustanna.net,resources=staticsites/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=drivethru.notjustanna.net,resources=staticsites/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=traefik.io,resources=ingressroutes,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the entry point invoked by controller-runtime.
func (r *StaticSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var site dtv1alpha1.StaticSite
	if err := r.Get(ctx, req.NamespacedName, &site); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !site.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &site)
	}

	if !controllerutil.ContainsFinalizer(&site, dtv1alpha1.Finalizer) {
		controllerutil.AddFinalizer(&site, dtv1alpha1.Finalizer)
		if err := r.Update(ctx, &site); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{Requeue: true}, nil
	}

	bucketID, err := r.Garage.EnsureBucket(ctx, site.Spec.Host)
	if err != nil {
		return r.fail(ctx, &site, "EnsureBucket", err)
	}
	if err := r.Garage.EnableWebsite(ctx, bucketID); err != nil {
		return r.fail(ctx, &site, "EnableWebsite", err)
	}

	secretName := ""
	if !site.Spec.GeneratedSecretName.IsDisabled() {
		secretName = site.Spec.GeneratedSecretName.NameOrDefault(site.Name + "-s3")
		if err := r.ensureCredentials(ctx, &site, bucketID, secretName); err != nil {
			return r.fail(ctx, &site, "EnsureCredentials", err)
		}
	}

	if err := r.ensureIngressRoute(ctx, &site); err != nil {
		return r.fail(ctx, &site, "EnsureIngressRoute", err)
	}

	site.Status.Ready = true
	site.Status.BucketName = site.Spec.Host
	site.Status.SecretName = secretName
	setCondition(&site, metav1.ConditionTrue, "Reconciled", "all subresources ready")
	if err := r.Status().Update(ctx, &site); err != nil {
		logger.Error(err, "update status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *StaticSiteReconciler) ensureCredentials(ctx context.Context, site *dtv1alpha1.StaticSite, bucketID, secretName string) error {
	keyName := "drivethru-" + site.Spec.Host

	info, created, err := r.Garage.EnsureKey(ctx, keyName)
	if err != nil {
		return fmt.Errorf("ensure key: %w", err)
	}
	if err := r.Garage.AllowKeyOnBucket(ctx, bucketID, info.AccessKeyID); err != nil {
		return fmt.Errorf("allow key on bucket: %w", err)
	}

	desired := map[string][]byte{
		"AWS_ACCESS_KEY_ID":     []byte(info.AccessKeyID),
		"BUCKET_NAME":           []byte(site.Spec.Host),
		"AWS_DEFAULT_REGION":    []byte("garage"),
	}
	if r.Cfg.GarageS3Endpoint != "" {
		desired["AWS_ENDPOINT_URL"] = []byte(r.Cfg.GarageS3Endpoint)
	}
	if created {
		desired["AWS_SECRET_ACCESS_KEY"] = []byte(info.SecretAccessKey)
	}

	var existing corev1.Secret
	err = r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: site.Namespace}, &existing)
	switch {
	case apierrors.IsNotFound(err):
		if !created {
			return fmt.Errorf("garage key %q exists but Secret %q is missing; recreate the StaticSite or delete the Garage key manually",
				keyName, secretName)
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: site.Namespace,
			},
			Data: desired,
		}
		if err := controllerutil.SetControllerReference(site, secret, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, secret)
	case err != nil:
		return err
	}

	for k, v := range desired {
		existing.Data[k] = v
	}
	if err := controllerutil.SetControllerReference(site, &existing, r.Scheme); err != nil {
		return err
	}
	return r.Update(ctx, &existing)
}

func (r *StaticSiteReconciler) ensureIngressRoute(ctx context.Context, site *dtv1alpha1.StaticSite) error {
	desired := traefik.BuildIngressRoute(
		site.Name,
		site.Namespace,
		site.Spec.Host,
		r.Cfg.TraefikEntrypoint,
		r.Cfg.TraefikCertResolver,
		r.Cfg.GarageServiceName,
		r.Cfg.GarageServiceNamespace,
		r.Cfg.GarageServicePort,
	)
	if err := controllerutil.SetControllerReference(site, desired, r.Scheme); err != nil {
		return err
	}

	var existing traefik.IngressRoute
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, &existing)
	switch {
	case apierrors.IsNotFound(err):
		return r.Create(ctx, desired)
	case err != nil:
		return err
	}
	existing.Spec = desired.Spec
	existing.OwnerReferences = desired.OwnerReferences
	return r.Update(ctx, &existing)
}

func (r *StaticSiteReconciler) reconcileDelete(ctx context.Context, site *dtv1alpha1.StaticSite) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(site, dtv1alpha1.Finalizer) {
		return ctrl.Result{}, nil
	}

	if !site.Spec.Retain {
		bucketID, bucketFound, err := r.Garage.LookupBucket(ctx, site.Spec.Host)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("lookup bucket: %w", err)
		}

		keyName := "drivethru-" + site.Spec.Host
		accessKeyID, keyFound, err := r.Garage.LookupKey(ctx, keyName)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("lookup key: %w", err)
		}

		if keyFound {
			if bucketFound {
				_ = r.Garage.DenyKeyOnBucket(ctx, bucketID, accessKeyID)
			}
			if err := r.Garage.DeleteKey(ctx, accessKeyID); err != nil {
				return ctrl.Result{}, fmt.Errorf("delete key: %w", err)
			}
		}

		if bucketFound {
			if err := r.Garage.DeleteBucket(ctx, bucketID); err != nil {
				return ctrl.Result{}, fmt.Errorf("delete bucket: %w", err)
			}
		}
	}

	// Secret + IngressRoute have owner refs and are GC'd by the apiserver; no
	// need to delete them explicitly.

	controllerutil.RemoveFinalizer(site, dtv1alpha1.Finalizer)
	if err := r.Update(ctx, site); err != nil {
		return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *StaticSiteReconciler) fail(ctx context.Context, site *dtv1alpha1.StaticSite, reason string, err error) (ctrl.Result, error) {
	site.Status.Ready = false
	setCondition(site, metav1.ConditionFalse, reason, err.Error())
	if uerr := r.Status().Update(ctx, site); uerr != nil {
		log.FromContext(ctx).Error(uerr, "status update on failure")
	}
	return ctrl.Result{}, err
}

func setCondition(site *dtv1alpha1.StaticSite, status metav1.ConditionStatus, reason, msg string) {
	cond := metav1.Condition{
		Type:               dtv1alpha1.ConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            msg,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: site.Generation,
	}
	for i, c := range site.Status.Conditions {
		if c.Type == cond.Type {
			if c.Status == cond.Status && c.Reason == cond.Reason {
				cond.LastTransitionTime = c.LastTransitionTime
			}
			site.Status.Conditions[i] = cond
			return
		}
	}
	site.Status.Conditions = append(site.Status.Conditions, cond)
}

// SetupWithManager wires the reconciler into a manager.
func (r *StaticSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dtv1alpha1.StaticSite{}).
		Owns(&corev1.Secret{}).
		Owns(&traefik.IngressRoute{}).
		Complete(r)
}
