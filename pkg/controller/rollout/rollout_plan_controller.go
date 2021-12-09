/*
Copyright 2021 The KubeVela Authors.

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
	"fmt"
	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/controller/rollout/workloads"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"reflect"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//TODO: plan 被修改后，batch 不能到下一环节，不然会被 pauseSecond block 住

// ReleaseReconcileRequeueTime is the default time to check back if we still have work to do
const ReleaseReconcileRequeueTime = 5 * time.Second

// Controller is the controller that controls the release plan resource
type Controller struct {
	client   client.Client
	recorder record.EventRecorder

	release        *v1alpha1.BatchRelease
	releasePlan    *v1alpha1.ReleasePlan
	releaseStatus  *v1alpha1.BatchReleaseStatus
	targetWorkload *apps.Deployment
}

// NewReleasePlanController creates a RolloutPlanController
func NewReleasePlanController(client client.Client, recorder record.EventRecorder, release *v1alpha1.BatchRelease,
	releasePlan *v1alpha1.ReleasePlan, releaseStatus *v1alpha1.BatchReleaseStatus) *Controller {
	initializedReleaseStatus := releaseStatus
	if len(initializedReleaseStatus.ReleasingBatchState) == 0 {
		initializedReleaseStatus.ReleasingBatchState = v1alpha1.BatchInitializingState
	}
	return &Controller{
		client:        client,
		recorder:      recorder,
		release:       release,
		releasePlan:   releasePlan,
		releaseStatus: initializedReleaseStatus.DeepCopy(),
	}
}

// Reconcile reconciles a release plan
func (r *Controller) Reconcile(ctx context.Context) (reconcile.Result, *v1alpha1.BatchReleaseStatus) {
	status := r.releaseStatus
	stopReconcile := reconcile.Result{}
	retryAfterShortDuration := reconcile.Result{RequeueAfter: 500 * time.Microsecond}
	retryAfterLongDuration := reconcile.Result{RequeueAfter: 5 * time.Second}

	klog.V(3).InfoS("Reconcile the release plan", "release status", r.releaseStatus,
		"target workload", r.release.Spec.TargetRef.Name)

	klog.V(3).InfoS("release status", "release state", r.releaseStatus.ReleasingState, "batch rolling state",
		r.releaseStatus.ReleasingBatchState, "current batch", r.releaseStatus.CurrentBatch, "upgraded Replicas",
		r.releaseStatus.UpgradedReplicas, "ready Replicas", r.releaseStatus.UpgradedReadyReplicas)

	workloadController, err := r.GetWorkloadController()
	if err != nil {
		status.RolloutFailed(err.Error())
		r.recorder.Event(r.release, v1.EventTypeWarning, "UnsupportedWorkload", err.Error())
		return stopReconcile, status
	}

	workload, changed, reconciling, err := workloadController.UpdateRevisionChangedDuringRelease(ctx)
	switch {
	case client.IgnoreNotFound(err) != nil:
		r.recorder.Event(r.release, v1.EventTypeWarning, "GetWorkloadFailed", err.Error())
		return retryAfterShortDuration, status

	case workload == nil:
		r.recorder.Event(r.release, v1.EventTypeWarning, "WorkloadGone", "workload has been deleted")
		status.StateTransition(v1alpha1.RollingFailedEvent)
		return stopReconcile, status

	case reconciling:
		klog.V(3).Infof("Workload is being reconciled by its controller, wait for reconcile done")
		return retryAfterShortDuration, status

	case changed:
		if succeed := workloadController.Finalize(ctx, true); !succeed {
			return retryAfterShortDuration, status
		}
		klog.Warningf("Workload UpdateRevision was changed during releasing")
		status.ResetStatus()
		return retryAfterShortDuration, status
	}

	changed, _ = workloadController.ReplicasChangedDuringRelease(ctx)
	if changed {
		if succeed := workloadController.Finalize(ctx, false); !succeed {
			return retryAfterShortDuration, status
		}
		message := "workload replicas changed during release, try to restart the release plan"
		klog.Warning(message)
		r.recorder.Eventf(r.release, v1.EventTypeWarning, "ReplicasChanged", message)
		status.RolloutRetry(message)
		status.ResetStatus()
		return retryAfterShortDuration, status
	}

	if r.releaseStatus.ObservedReleasePlanHash != "" && r.releaseStatus.ObservedReleasePlanHash != hashReleasePlanBatches(r.releasePlan) {
		if succeed := workloadController.Finalize(ctx, false); !succeed {
			return retryAfterShortDuration, status
		}
		message := "release plan has changed, try to restart the release plan"
		klog.Warning(message)
		r.recorder.Eventf(r.release, v1.EventTypeWarning, "ReleasePlanChanged", message)
		status.RolloutRetry(message)
		status.ResetStatus()
		return retryAfterShortDuration, status
	}

	if status.CurrentBatch >= 1 && r.releaseStatus.ReleasingState == v1alpha1.RollingInBatchesState {
		lastBatch := r.releasePlan.Batches[status.CurrentBatch-1]
		waitDuration := time.Duration(lastBatch.PauseSeconds) * time.Second
		if status.LastBatchFinalizedTime.Time.Add(waitDuration).After(time.Now()) {
			restDuration := status.LastBatchFinalizedTime.Time.Add(waitDuration).Sub(time.Now())
			klog.V(3).Infof("BatchRelease %v/%v paused and will continue to reconcile after %v", r.release.Namespace, r.release.Name, restDuration)
			return reconcile.Result{RequeueAfter: restDuration}, status
		}
	}

	switch status.ReleasingState {
	case v1alpha1.VerifyingSpecState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", v1alpha1.VerifyingSpecState)

		verified, err := workloadController.VerifySpec(ctx)
		if err != nil {
			// we can fail it right away, everything after initialized need to be finalized
			status.RolloutFailed(err.Error())
		} else if verified {
			status.StateTransition(v1alpha1.RollingSpecVerifiedEvent)
		}

	case v1alpha1.InitializingState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", v1alpha1.InitializingState)

		initialized, err := workloadController.Initialize(ctx)
		if err != nil {
			status.RolloutFailing(err.Error())
		} else if initialized {
			status.StateTransition(v1alpha1.RollingInitializedEvent)
		}

	case v1alpha1.RollingInBatchesState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", v1alpha1.RollingInBatchesState)
		r.reconcileBatchInRolling(ctx, workloadController)

	case v1alpha1.RolloutFailingState, v1alpha1.RolloutAbandoningState, v1alpha1.RolloutDeletingState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", fmt.Sprintf("%s/%s/%s",
			v1alpha1.RolloutFailingState, v1alpha1.RolloutAbandoningState, v1alpha1.RolloutDeletingState))
		if succeed := workloadController.Finalize(ctx, false); succeed {
			r.finalizeRollout(ctx)
		}

	case v1alpha1.FinalisingState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", v1alpha1.FinalisingState)

		if succeed := workloadController.Finalize(ctx, true); succeed {
			r.finalizeRollout(ctx)
		}

	case v1alpha1.RolloutSucceedState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", v1alpha1.RolloutSucceedState)
		r.releaseStatus.StableRevision = r.releaseStatus.UpdateRevision
		r.releaseStatus.LastUpdateRevision = r.releaseStatus.UpdateRevision

	case v1alpha1.RolloutFailedState:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", v1alpha1.RolloutFailedState)
		// Nothing to do

	default:
		klog.V(3).Infof("ReleasePlan State Machine into %s state", "Unknown")

		panic(fmt.Sprintf("illegal release status %+v", status))
	}

	switch status.ReleasingState {
	case v1alpha1.RollingInBatchesState:
		if status.ReleasingBatchState == v1alpha1.BatchVerifyingState {
			return retryAfterLongDuration, status
		}
		return retryAfterShortDuration, status
	case v1alpha1.InitializingState, v1alpha1.RolloutFailingState, v1alpha1.RolloutAbandoningState,
		v1alpha1.RolloutDeletingState, v1alpha1.FinalisingState:
		return retryAfterShortDuration, status
	case v1alpha1.RolloutFailedState, v1alpha1.RolloutSucceedState:
		return stopReconcile, status
	}
	return retryAfterLongDuration, status
}

// reconcile logic when we are in the middle of release, we have to go through finalizing state before succeed or fail
func (r *Controller) reconcileBatchInRolling(ctx context.Context, workloadController workloads.WorkloadController) {
	if r.releasePlan.Paused {
		r.recorder.Event(r.release, v1.EventTypeNormal, "RolloutPaused", "Rollout paused")
		r.releaseStatus.SetConditions(v1alpha1.NewPositiveCondition(v1alpha1.BatchPaused))
		return
	}

	switch r.releaseStatus.ReleasingBatchState {
	case v1alpha1.BatchInitializingState:
		klog.V(3).Infof("ReleaseBatch State Machine into %s state", v1alpha1.BatchInitializingState)

		r.initializeOneBatch(ctx)
		fallthrough

	case v1alpha1.BatchInRollingState:
		klog.V(3).Infof("ReleaseBatch State Machine into %s state", v1alpha1.BatchInRollingState)

		//  still rolling the batch, the batch rolling is not completed yet
		upgradeDone, err := workloadController.RolloutOneBatchPods(ctx)
		if err != nil {
			r.releaseStatus.RolloutFailing(err.Error())
		} else if upgradeDone {
			r.releaseStatus.StateTransition(v1alpha1.RolloutOneBatchEvent)
		}

	case v1alpha1.BatchVerifyingState:
		klog.V(3).Infof("ReleaseBatch State Machine into %s state", v1alpha1.BatchVerifyingState)

		// verifying if the application is ready to roll
		// need to check if they meet the availability requirements in the release spec.
		// TODO: evaluate any metrics/analysis
		// TODO: We may need to go back to release again if the size of the resource can change behind our back
		verified, err := workloadController.CheckOneBatchPods(ctx)
		if err != nil {
			r.releaseStatus.RolloutFailing(err.Error())
		} else if verified {
			r.releaseStatus.StateTransition(v1alpha1.OneBatchAvailableEvent)
		}

	case v1alpha1.BatchFinalizingState:
		klog.V(3).Infof("ReleaseBatch State Machine into %s state", v1alpha1.BatchFinalizingState)

		// finalize one batch
		finalized, err := workloadController.FinalizeOneBatch(ctx)
		if err != nil {
			r.releaseStatus.RolloutFailing(err.Error())
		} else if finalized {
			r.finalizeOneBatch(ctx)
		}

	case v1alpha1.BatchReadyState:
		klog.V(3).Infof("ReleaseBatch State Machine into %s state", v1alpha1.BatchReadyState)

		// all the pods in the are upgraded and their state are ready
		// wait to move to the next batch if there are any
		r.tryMovingToNextBatch(workloadController)

	default:
		klog.V(3).Infof("ReleaseBatch State Machine into %s state", "Unknown")
		panic(fmt.Sprintf("illegal status %+v", r.releaseStatus))
	}
}

// all the common initialize work before we release
// TODO: fail the release if the webhook call is explicitly rejected (through http status code)
func (r *Controller) initializeRollout(ctx context.Context) error {
	return nil
}

// all the common initialize work before we release one batch of resources
func (r *Controller) initializeOneBatch(ctx context.Context) {
	r.releaseStatus.StateTransition(v1alpha1.InitializedOneBatchEvent)
}

// check if we can move to the next batch
func (r *Controller) tryMovingToNextBatch(controller workloads.WorkloadController) {
	if r.releasePlan.BatchPartition == nil || *r.releasePlan.BatchPartition > r.releaseStatus.CurrentBatch {
		klog.V(3).InfoS("ready to release the next batch", "current batch", r.releaseStatus.CurrentBatch)
		r.releaseStatus.StateTransition(v1alpha1.BatchRolloutApprovedEvent)
		r.releaseStatus.CurrentBatch = controller.MoveToSuitableBatch(context.Background())
	} else {
		klog.V(3).InfoS("the current batch is waiting to move on", "current batch",
			r.releaseStatus.CurrentBatch)
	}
	// record this batch finalizedTime
	r.releaseStatus.LastBatchFinalizedTime.Time = time.Now()
}

func (r *Controller) finalizeOneBatch(ctx context.Context) {
	// calculate the next phase
	currentBatch := int(r.releaseStatus.CurrentBatch)
	if currentBatch == len(r.releasePlan.Batches)-1 {
		// this is the last batch, mark the release finalized
		r.releaseStatus.StateTransition(v1alpha1.AllBatchFinishedEvent)
		//r.recorder.Event(r.parentController, event.Normal("All batches rolled out",
		//	fmt.Sprintf("upgrade pod = %d, total ready pod = %d", r.rolloutStatus.UpgradedReplicas,
		//		r.rolloutStatus.UpgradedReadyReplicas)))
	} else {
		klog.V(3).InfoS("finished one batch release", "current batch", r.releaseStatus.CurrentBatch)
		//r.recorder.Event(r.parentController, event.Normal("Batch Finalized",
		//fmt.Sprintf("Batch %d is finalized and ready to go", r.rolloutStatus.CurrentBatch)))
		r.releaseStatus.StateTransition(v1alpha1.FinishedOneBatchEvent)
	}
}

// all the common finalize work after we release
func (r *Controller) finalizeRollout(ctx context.Context) {
	r.releaseStatus.StateTransition(v1alpha1.RollingFinalizedEvent)
}

// GetWorkloadController pick the right workload controller to work on the workload
func (r *Controller) GetWorkloadController() (workloads.WorkloadController, error) {
	targetRef := r.release.Spec.TargetRef

	targetKey := types.NamespacedName{
		Namespace: r.release.Namespace,
		Name:      targetRef.Name,
	}

	switch targetRef.APIVersion {
	case v1alpha1.GroupVersion.String():
		if targetRef.Kind == reflect.TypeOf(v1alpha1.CloneSet{}).Name() {
			klog.InfoS("using cloneset batch release controller for this batch release", "workload name", targetKey.Name, "namespace", targetKey.Namespace)
			return workloads.NewCloneSetRolloutController(r.client, r.recorder, r.release, r.releasePlan, r.releaseStatus, targetKey), nil
		}
	case apps.SchemeGroupVersion.String():
		if targetRef.Kind == reflect.TypeOf(apps.Deployment{}).Name() {
			klog.InfoS("using deployment batch release controller for this batch release", "workload name", targetKey.Name, "namespace", targetKey.Namespace)
			return workloads.NewDeploymentRolloutController(r.client, r.recorder, r.release, r.releasePlan, r.releaseStatus, targetKey), nil
		}
		/*
			if targetRef.Kind == reflect.TypeOf(apps.StatefulSet{}).Name() {
				klog.InfoS("using statefulset batch release controller for this batch release", "workload name", targetKey.Name, "namespace", targetKey.Namespace)
				return workloads.NewCloneSetRolloutController(r.Client, r.recorder, release, &release.Spec.ReleasePlan, &release.Status, targetKey), nil
			}
		*/
	}
	return nil, fmt.Errorf("the workload `%v/%v` is not supported", targetRef.APIVersion, targetRef.Kind)
}
