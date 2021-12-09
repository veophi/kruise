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

package workloads

import (
	"context"
	"fmt"
	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CloneSetRolloutController is responsible for handling rollout CloneSet type of workloads
type CloneSetRolloutController struct {
	cloneSetController
	clone *v1alpha1.CloneSet
}

//TODO: scale during releasing: workload replicas changed -> Finalising CloneSet with Paused=true

// NewCloneSetRolloutController creates a new CloneSet rollout controller
func NewCloneSetRolloutController(client client.Client, recorder record.EventRecorder, rollout *v1alpha1.BatchRelease,
	rolloutSpec *v1alpha1.ReleasePlan, rolloutStatus *v1alpha1.BatchReleaseStatus,
	targetNamespacedName types.NamespacedName) *CloneSetRolloutController {
	return &CloneSetRolloutController{
		cloneSetController: cloneSetController{
			workloadController: workloadController{
				client:           client,
				recorder:         recorder,
				parentController: rollout,
				releasePlan:      rolloutSpec,
				releaseStatus:    rolloutStatus,
			},
			targetNamespacedName: targetNamespacedName,
		},
	}
}

// VerifySpec verifies that the rollout resource is consistent with the rollout spec
func (c *CloneSetRolloutController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error

	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			c.recorder.Event(c.parentController, v1.EventTypeWarning, "VerifyFailed", verifyErr.Error())
		}
	}()

	if err := c.fetchCloneSet(ctx); err != nil {
		c.releaseStatus.RolloutRetry(err.Error())
		return false, nil
	}

	if c.clone.Status.ObservedGeneration != c.clone.Generation {
		klog.Warningf("CloneSet is still reconciling, need to be paused or stable")
		return false, nil
	}

	if *c.clone.Spec.Replicas != c.clone.Status.Replicas {
		klog.Warningf("CloneSet is still scaling, need to be paused or stable")
		verifyErr = fmt.Errorf("cloneset is still scaling, need to be paused or stable")
		return false, verifyErr
	}

	if c.clone.Status.UpdateRevision == c.clone.Status.CurrentRevision {
		klog.Warningf("CloneSet is rolling back, need to be paused or stable")
		verifyErr = fmt.Errorf("cloneset maybe rollback, release plan should be paused")
		return false, verifyErr
	}

	if isHealthy, err := cloneSetIsHealthyState(c.clone); !isHealthy || err != nil {
		klog.Warningf("CloneSet is not healthy or has been stable, no need to release")
		verifyErr = err
		return false, verifyErr
	}

	c.recordCloneSetRevisionAndReplicas()
	klog.V(3).Infof("Verified Successfully, Status %+v", c.releaseStatus)
	c.recorder.Event(c.parentController, v1.EventTypeNormal, "RolloutVerified", "ReleasePlan and the CloneSet resource are verified")
	return true, nil
}

// Initialize makes sure that the source and target CloneSet is under our control
func (c *CloneSetRolloutController) Initialize(ctx context.Context) (bool, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		c.releaseStatus.RolloutRetry(err.Error())
		return false, nil
	}

	if _, err := c.claimCloneSet(ctx, c.clone); err != nil {
		return false, nil
	}

	c.recorder.Event(c.parentController, v1.EventTypeNormal, "Rollout Initialized", "Rollout resource are initialized")
	return true, nil
}

// RolloutOneBatchPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly
func (c *CloneSetRolloutController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		c.releaseStatus.RolloutRetry(err.Error())
		return false, nil
	}

	updateSize := c.calculateCurrentTarget(c.releaseStatus.ObservedWorkloadReplicas)
	stableSize := c.calculateCurrentSource(c.releaseStatus.ObservedWorkloadReplicas)
	workloadPartition, _ := intstr.GetValueFromIntOrPercent(c.clone.Spec.UpdateStrategy.Partition,
		int(c.releaseStatus.ObservedWorkloadReplicas), true)

	if c.clone.Status.UpdatedReplicas >= updateSize && int32(workloadPartition) <= stableSize {
		klog.V(3).InfoS("upgraded one batch, but no need to update partition of cloneset", "current batch",
			c.releaseStatus.CurrentBatch, "real updateRevision size", c.clone.Status.UpdatedReplicas)
		return true, nil
	}

	if err := c.patchCloneSetPartition(ctx, c.clone, stableSize); err != nil {
		return false, nil
	}

	klog.V(3).InfoS("upgraded one batch", "current batch", c.releaseStatus.CurrentBatch, "updateRevision size", updateSize)
	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Rollout", "Finished submitting all upgrade quests for batch %d", c.releaseStatus.CurrentBatch)
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (c *CloneSetRolloutController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		return false, nil
	}

	if c.clone.Status.ObservedGeneration != c.clone.Generation {
		return false, nil
	}

	updatePodCount := c.clone.Status.UpdatedReplicas
	stablePodCount := c.clone.Status.Replicas - updatePodCount
	readyUpdatePodCount := c.clone.Status.UpdatedReadyReplicas
	currentBatch := c.releasePlan.Batches[c.releaseStatus.CurrentBatch]
	updateGoal := c.calculateCurrentTarget(c.releaseStatus.ObservedWorkloadReplicas)
	stableGoal := c.calculateCurrentSource(c.releaseStatus.ObservedWorkloadReplicas)

	c.releaseStatus.UpgradedReplicas = updatePodCount
	c.releaseStatus.UpgradedReadyReplicas = readyUpdatePodCount

	rolloutStrategy := v1alpha1.IncreaseFirstReleaseStrategyType
	if len(c.releasePlan.Strategy) != 0 {
		rolloutStrategy = c.releasePlan.Strategy
	}

	maxUnavailable := 0
	if currentBatch.MaxUnavailable != nil {
		maxUnavailable, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(c.releaseStatus.ObservedWorkloadReplicas), true)
	}

	klog.InfoS("checking the batch releasing progress", "current batch", c.releaseStatus.CurrentBatch,
		"target pod ready count", readyUpdatePodCount, "source pod count", stablePodCount,
		"max unavailable pod allowed", maxUnavailable, "target goal", updateGoal, "source goal", stableGoal,
		"rolloutStrategy", rolloutStrategy)

	if updateGoal > updatePodCount || stableGoal < stablePodCount || readyUpdatePodCount+int32(maxUnavailable) < updateGoal {
		klog.InfoS("the batch is not ready yet", "current batch", c.releaseStatus.CurrentBatch)
		c.releaseStatus.RolloutRetry(fmt.Sprintf(
			"the batch %d is not ready yet with %d target pods ready and %d source pods with %d unavailable allowed",
			c.releaseStatus.CurrentBatch, readyUpdatePodCount, stablePodCount, maxUnavailable))
		return false, nil
	}

	klog.InfoS("all pods in current batch are ready", "current batch", c.releaseStatus.CurrentBatch)
	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Available", "Batch %d is available", c.releaseStatus.CurrentBatch)
	return true, nil
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (c *CloneSetRolloutController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	if c.releasePlan.BatchPartition != nil && *c.releasePlan.BatchPartition < c.releaseStatus.CurrentBatch {
		err := fmt.Errorf("the current batch value in the status is greater than the batch partition")
		klog.ErrorS(err, "we have moved past the user defined partition", "user specified batch partition",
			*c.releasePlan.BatchPartition, "current batch we are working on", c.releaseStatus.CurrentBatch)
		return false, err
	}

	if c.clone.Status.Replicas != c.releaseStatus.ObservedWorkloadReplicas {
		updatePodCount := c.clone.Status.UpdatedReplicas
		stablePodCount := c.clone.Status.Replicas - c.clone.Status.UpdatedReplicas
		err := fmt.Errorf("CloneSet replicas don't match ObservedWorkloadReplicas, sourceTarget = %d, targetTarget = %d, "+
			"rolloutTargetSize = %d", stablePodCount, updatePodCount, c.releaseStatus.ObservedWorkloadReplicas)
		klog.ErrorS(err, "the batch is not valid", "current batch", c.releaseStatus.CurrentBatch)
		return false, nil
	}

	return true, nil
}

// Finalize makes sure the CloneSet is all upgraded
func (c *CloneSetRolloutController) Finalize(ctx context.Context, succeed bool) bool {
	if err := c.fetchCloneSet(ctx); err != nil {
		return false
	}

	if _, err := c.releaseCloneSet(ctx, c.clone, succeed); err != nil {
		return false
	}

	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Rollout Finalized", "Rollout resource are finalized, succeed := %t", succeed)
	return true
}

func (c *CloneSetRolloutController) UpdateRevisionChangedDuringRelease(ctx context.Context) (runtime.Object, bool, bool, error) {
	if err := c.fetchCloneSet(ctx); err != nil {
		return nil, false, false, err
	}

	if c.releaseStatus.ReleasingState == v1alpha1.VerifyingSpecState && c.releaseStatus.UpdateRevision == "" {
		return c.clone, false, false, nil
	}

	if c.clone.Status.ObservedGeneration != c.clone.Generation {
		klog.Warningf("CloneSet is still reconciling, waiting for it to complete, generation: %v, observed: %v", c.clone.Generation, c.clone.Status.ObservedGeneration)
		return c.clone, false, true, nil
	}

	if c.clone.Status.UpdateRevision != c.releaseStatus.UpdateRevision {
		klog.Warningf("Release Controller Observe UpdateRevision Changed: %v -> %v", c.releaseStatus.UpdateRevision, c.clone.Status.UpdateRevision)
		return c.clone, true, false, nil
	}
	return c.clone, false, false, nil
}

func (c *CloneSetRolloutController) ReplicasChangedDuringRelease(ctx context.Context) (bool, error) {
	if c.releaseStatus.ObservedWorkloadReplicas == -1 {
		return false, nil
	}
	if c.releaseStatus.ReleasingState == v1alpha1.VerifyingSpecState && c.releaseStatus.UpdateRevision == "" {
		return false, nil
	}
	if c.clone == nil {
		if err := c.fetchCloneSet(ctx); err != nil {
			return false, err
		}
	}
	if *c.clone.Spec.Replicas == c.releaseStatus.ObservedWorkloadReplicas {
		return false, nil
	}
	klog.Warningf("CloneSet replicas changed during releasing, should pause and wait for it to complete")
	return true, nil
}

func (c *CloneSetRolloutController) MoveToSuitableBatch(ctx context.Context) int32 {
	if c.clone == nil {
		if err := c.fetchCloneSet(ctx); err != nil {
			return c.releaseStatus.CurrentBatch
		}
	}

	workloadPartition, _ := intstr.GetValueFromIntOrPercent(c.clone.Spec.UpdateStrategy.Partition,
		int(c.releaseStatus.ObservedWorkloadReplicas), true)

	batchCount := len(c.releasePlan.Batches)
	for i := 0; i < batchCount; i++ {
		updateGoal := calculateNewBatchTarget(c.releasePlan, int(c.releaseStatus.ObservedWorkloadReplicas), i)
		stableGoal := int(c.releaseStatus.ObservedWorkloadReplicas) - updateGoal
		if int(c.clone.Status.UpdatedReplicas) < updateGoal || workloadPartition > stableGoal {
			return int32(i)
		}
	}
	return c.releaseStatus.CurrentBatch
}

/* ----------------------------------
The functions below are helper functions
------------------------------------- */
func (c *CloneSetRolloutController) fetchCloneSet(ctx context.Context) error {
	c.clone = &v1alpha1.CloneSet{}
	if err := c.client.Get(context.TODO(), c.targetNamespacedName, c.clone); err != nil {
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, v1.EventTypeWarning, "GetCloneSetFailed", err.Error())
		}
		return err
	}
	return nil
}

// calculateRolloutTotalSize fetches the CloneSet and returns the replicas (not the actual number of pods)
func (c *CloneSetRolloutController) getCloneSetReplicas() (int32, error) {
	return getCloneSetReplicas(c.clone), nil
}

// check if the replicas in all the rollout batches add up to the right number
func (c *CloneSetRolloutController) verifyRolloutBatchReplicaValue(totalReplicas int32) error {
	// use a common function to check if the sum of all the batches can match the CloneSet size
	return verifyBatchesWithRollout(c.releasePlan, totalReplicas)
}

// the target workload size for the current batch
func (c *CloneSetRolloutController) calculateCurrentTarget(totalSize int32) int32 {
	targetSize := int32(calculateNewBatchTarget(c.releasePlan, int(totalSize), int(c.releaseStatus.CurrentBatch)))
	klog.InfoS("Calculated the number of pods in the target CloneSet after current batch",
		"current batch", c.releaseStatus.CurrentBatch, "workload updateRevision size", targetSize)
	return targetSize
}

// the source workload size for the current batch
func (c *CloneSetRolloutController) calculateCurrentSource(totalSize int32) int32 {
	sourceSize := totalSize - c.calculateCurrentTarget(totalSize)
	klog.InfoS("Calculated the number of pods in the source CloneSet after current batch",
		"current batch", c.releaseStatus.CurrentBatch, "workload stableRevision size", sourceSize)
	return sourceSize
}

func (c *CloneSetRolloutController) recordCloneSetRevisionAndReplicas() {
	c.releaseStatus.ObservedWorkloadReplicas = *c.clone.Spec.Replicas
	c.releaseStatus.StableRevision = c.clone.Status.CurrentRevision
	c.releaseStatus.UpdateRevision = c.clone.Status.UpdateRevision
}

func cloneSetIsHealthyState(clone *v1alpha1.CloneSet) (bool, error) {
	switch {
	case !clone.Spec.UpdateStrategy.Paused:
		return false, fmt.Errorf("CloneSet.UpdateStrategy.Paused should be true before releasing")
	case clone.Status.UpdatedReplicas == *clone.Spec.Replicas:
		return true, fmt.Errorf("updateRevision has been promoted, no need to release")
	}
	return true, nil
}
