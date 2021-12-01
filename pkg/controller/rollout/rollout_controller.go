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
	"encoding/json"
	"flag"
	"github.com/openkruise/kruise/pkg/controller/rollout/workloads"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	flag.IntVar(&concurrentReconciles, "rollout-workers", concurrentReconciles, "Max concurrent workers for Rollout controller.")
}

var (
	concurrentReconciles = 3
	//controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("Rollout")
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Rollout Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	//if !utildiscovery.DiscoverGVK(controllerKind) {
	//	return nil
	//}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor("rollout-controller")
	cli := util.NewClientFromManager(mgr, "rollout-controller")
	return &ReconcileRollout{
		Client:   cli,
		scheme:   mgr.GetScheme(),
		recorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("rollout-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to Rollout
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.Rollout{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*appsv1alpha1.Rollout)
			new := e.ObjectNew.(*appsv1alpha1.Rollout)
			if !reflect.DeepEqual(old.Spec, new.Spec) {
				klog.V(3).Infof("Observed updated Spec for Rollout: %s/%s", new.Namespace, new.Name)
				return true
			}
			return false
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to Deployment
	err = c.Watch(&source.Kind{Type: &apps.Deployment{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*apps.Deployment)
			new := e.ObjectNew.(*apps.Deployment)
			if !reflect.DeepEqual(old.Spec, new.Spec) {
				klog.V(3).Infof("Observed updated Spec for Deployment: %s/%s", new.Namespace, new.Name)
				return true
			}
			return false
		},
	})
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

// +kubebuilder:rbac:groups=apps.kruise.io,resources=rollouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=rollouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a Rollout object and makes changes based on the state read
// and what is in the Rollout.Spec
func (r *ReconcileRollout) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rollout := new(appsv1alpha1.Rollout)
	if err := r.Get(context.TODO(), req.NamespacedName, rollout); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	klog.V(3).InfoS("start rollout reconcile", "rollout",
		klog.KRef(rollout.Namespace, rollout.Name), "rolling state", rollout.Status.RollingState)

	if len(rollout.Status.RollingState) == 0 {
		rollout.Status.ResetStatus()
	}

	deploymentController := workloads.NewDeploymentRolloutController(
		r.Client, r.recorder, rollout, rollout.Spec.RolloutPlan.DeepCopy(), rollout.Status.RolloutStatus.DeepCopy(), types.NamespacedName{
			Namespace: rollout.Namespace, Name: rollout.Spec.TargetRef.Name,
		})
	if err := deploymentController.FetchDeploymentAndItsReplicaSets(context.Background()); err != nil {
		klog.Errorf("Get deployment and its replicaset Failed, error: %v", err)
		return reconcile.Result{}, err
	}
	if deploymentController.GetUpdateRevision() != rollout.Status.UpdateRevision {
		rollout.Status.ResetStatus()
	}

	var err error
	var workload *apps.Deployment
	switch rollout.Status.RollingState {
	case appsv1alpha1.RolloutDeletingState:
		workload, err = r.checkWorkloadNotExist(rollout)
		if err != nil {
			return ctrl.Result{}, err
		}
		if workload == nil {
			klog.V(3).InfoS(" the target workload is gone, no need do anything", "rollout",
				klog.KRef(rollout.Namespace, rollout.Name), "rolling state", rollout.Status.RollingState)
			rollout.Status.StateTransition(appsv1alpha1.RollingFinalizedEvent)
			// update the appRollout status
		}
	case appsv1alpha1.LocatingTargetAppState:
		rollout.Status.StateTransition(appsv1alpha1.AppLocatedEvent)
	default:
		// we should do nothing
	}

	workload, err = r.getTargetRef(rollout)
	if err != nil {
		klog.Errorf("Get Target Workload Failed, error %v", err)
		return reconcile.Result{}, err
	} else if workload == nil {
		klog.V(3).InfoS(" the target workload is gone, no need do anything", "rollout",
			klog.KRef(rollout.Namespace, rollout.Name), "rolling state", rollout.Status.RollingState)
		rollout.Status.StateTransition(appsv1alpha1.RollingFinalizedEvent)
		return reconcile.Result{}, nil
	}

	rolloutPlanController := NewRolloutPlanController(r.Client, r.recorder, rollout, &rollout.Spec.RolloutPlan, &rollout.Status.RolloutStatus, workload)
	result, rolloutStatus := rolloutPlanController.Reconcile(context.Background())

	by, _ := json.Marshal(rolloutStatus)
	klog.V(3).Infof("Rollout Status %v", string(by))

	rollout.Status.RolloutStatus = *rolloutStatus
	return result, r.updateStatus(ctx, rollout)
}

func (r *ReconcileRollout) updateStatus(ctx context.Context, rollout *appsv1alpha1.Rollout) error {
	return r.Status().Update(ctx, rollout)
}

func (r *ReconcileRollout) checkWorkloadNotExist(rollout *appsv1alpha1.Rollout) (*apps.Deployment, error) {
	deploy := &apps.Deployment{}
	if err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: rollout.GetNamespace(), Name: rollout.Spec.TargetRef.Name}, deploy); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return deploy, nil
}

func (r *ReconcileRollout) getTargetRef(rollout *appsv1alpha1.Rollout) (*apps.Deployment, error) {
	key := types.NamespacedName{
		Namespace: rollout.Namespace,
		Name:      rollout.Spec.TargetRef.Name,
	}
	deploy := &apps.Deployment{}
	err := r.Get(context.TODO(), key, deploy)

	switch {
	case errors.IsNotFound(err):
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return deploy, nil
	}
}
