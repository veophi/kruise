/*
Copyright 2019 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rollout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "batchrelease-workers", concurrentReconciles, "Max concurrent workers for BatchRelease controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("BatchRelease")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Rollout Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("batchrelease-controller")
	cli := util.NewClientFromManager(mgr, "batchrelease-controller")
	return &ReconcileRollout{
		Client:   cli,
		scheme:   mgr.GetScheme(),
		recorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("batchrelease-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to BatchRelease
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.BatchRelease{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*appsv1alpha1.BatchRelease)
			new := e.ObjectNew.(*appsv1alpha1.BatchRelease)
			if !reflect.DeepEqual(old.Spec, new.Spec) {
				klog.V(3).Infof("Observed updated Spec for BatchRelease: %s/%s", new.Namespace, new.Name)
				return true
			}
			return false
		},
	})
	if err != nil {
		return err
	}

	// Watch changes to CloneSet
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.CloneSet{}}, &workloadEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}

	// Watch changes to Deployment
	err = c.Watch(&source.Kind{Type: &apps.Deployment{}}, &workloadEventHandler{Reader: mgr.GetCache()})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileRollout{}

// ReconcileRollout reconciles a Rollout object
type ReconcileRollout struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=batchreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=batchreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=clonesets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a Rollout object and makes changes based on the state read
// and what is in the Rollout.Spec
func (r *ReconcileRollout) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rollout := new(appsv1alpha1.BatchRelease)
	if err := r.Get(context.TODO(), req.NamespacedName, rollout); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	klog.V(3).InfoS("start batch release reconcile", "release",
		klog.KRef(rollout.Namespace, rollout.Name), "release state", rollout.Status.ReleasingState)

	if len(rollout.Status.ReleasingState) == 0 ||
		rollout.Status.ReleasingState == appsv1alpha1.LocatingTargetAppState {
		rollout.Status.ResetStatus()
	}

	rolloutPlanController := NewReleasePlanController(r.Client, r.recorder, rollout, &rollout.Spec.ReleasePlan, &rollout.Status)
	result, rolloutStatus := rolloutPlanController.Reconcile(context.Background())
	rolloutStatus.ObservedReleasePlanHash = hashReleasePlanBatches(&rollout.Spec.ReleasePlan)

	defer func() {
		klog.V(3).InfoS("Finished one round of reconciling release plan", "release state", rolloutStatus.ReleasingState,
			"batch rolling state", rolloutStatus.ReleasingBatchState, "current batch", rolloutStatus.CurrentBatch,
			"upgraded Replicas", rolloutStatus.UpgradedReplicas, "ready Replicas", rolloutStatus.UpgradedReadyReplicas,
			"reconcile result ", result)
	}()

	return result, r.updateStatus(ctx, rollout, rolloutStatus)
}

func (r *ReconcileRollout) updateStatus(ctx context.Context, release *appsv1alpha1.BatchRelease, newStatus *appsv1alpha1.BatchReleaseStatus) error {
	if !reflect.DeepEqual(release.Status, *newStatus) {
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			fetchedRelease := &appsv1alpha1.BatchRelease{}
			if getErr := r.Client.Get(context.TODO(), types.NamespacedName{
				Namespace: release.Namespace, Name: release.Name}, fetchedRelease); getErr != nil {
				return getErr
			}
			fetchedRelease.Status = *newStatus
			return r.Status().Update(ctx, fetchedRelease)
		})
	}
	return nil
}

func hashReleasePlanBatches(releasePlan *appsv1alpha1.ReleasePlan) string {
	by, _ := json.Marshal(releasePlan.Batches)
	md5Hash := sha256.Sum256(by)
	return hex.EncodeToString(md5Hash[:])
}
