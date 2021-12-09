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
	deploymentutil "github.com/openkruise/kruise/pkg/controller/rollout/workloads/copy_from_deployment/util"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	k8scontroller "k8s.io/kubernetes/pkg/controller"
	"k8s.io/utils/integer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
)

// DeploymentRolloutController is responsible for handling rollout deployment type of workloads
type DeploymentRolloutController struct {
	deploymentController
	deployment        *apps.Deployment
	targetReplicaSet  *apps.ReplicaSet
	sourceReplicaSets []*apps.ReplicaSet
}

// NewDeploymentRolloutController creates a new deployment rollout controller
func NewDeploymentRolloutController(client client.Client, recorder record.EventRecorder, rollout *v1alpha1.BatchRelease,
	rolloutSpec *v1alpha1.ReleasePlan, rolloutStatus *v1alpha1.BatchReleaseStatus,
	targetNamespacedName types.NamespacedName) *DeploymentRolloutController {
	return &DeploymentRolloutController{
		deploymentController: deploymentController{
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
func (c *DeploymentRolloutController) VerifySpec(ctx context.Context) (bool, error) {
	var verifyErr error

	defer func() {
		if verifyErr != nil {
			klog.Error(verifyErr)
			c.recorder.Event(c.parentController, v1.EventTypeWarning, "VerifyFailed", verifyErr.Error())
		}
	}()

	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		c.releaseStatus.RolloutRetry(err.Error())
		// do not fail the rollout just because we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	// check if the rollout spec is compatible with the current state
	deploymentReplicas, verifyErr := c.getDeploymentReplicas()
	if verifyErr != nil {
		return false, verifyErr
	}
	// record the size and we will use this value to drive the rest of the batches
	// we do not handle scale case in this controller
	c.releaseStatus.ObservedWorkloadReplicas = deploymentReplicas

	if !c.deployment.Spec.Paused && getDeploymentReplicas(c.deployment) != c.deployment.Status.Replicas {
		return false, fmt.Errorf("the deployment %s is still being reconciled, need to be paused or stable",
			c.deployment.GetName())
	}

	if achievedHealthy := c.checkHealthyAndRecordRevisions(); !achievedHealthy {
		klog.V(3).Infof("deployment has not achieve healthy state")
		return false, nil
	}

	// make sure that the updateRevision is different from what we have already done
	if (getReplicaSetsReplicas(c.sourceReplicaSets...) == 0 && *c.targetReplicaSet.Spec.Replicas == deploymentReplicas) ||
		(c.releaseStatus.StableRevision == c.releaseStatus.UpdateRevision) {
		return false, fmt.Errorf("there is no difference between the source and target")
	}

	// mark the rollout verified
	c.recorder.Event(c.parentController, v1.EventTypeNormal, "Rollout Verified", "Rollout spec and the Deployment resource are verified")
	// record the new pod template hash on success
	//c.rolloutStatus.NewPodTemplateIdentifier = targetHash
	return true, nil
}

// Initialize makes sure that the source and target deployment is under our control
func (c *DeploymentRolloutController) Initialize(ctx context.Context) (bool, error) {
	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		c.releaseStatus.RolloutRetry(err.Error())
		return false, nil
	}

	// claim deployment
	if _, err := c.claimDeployment(ctx, c.deployment, nil); err != nil {
		// nolint:nilerr
		return false, nil
	}

	// mark the rollout initialized
	c.recorder.Event(c.parentController, v1.EventTypeNormal, "Rollout Initialized", "Rollout resource are initialized")
	return true, nil
}

// RolloutOneBatchPods calculates the number of pods we can upgrade once according to the rollout spec
// and then set the partition accordingly
func (c *DeploymentRolloutController) RolloutOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		c.releaseStatus.RolloutRetry(err.Error())
		return false, nil
	}

	// get the rollout strategy
	rolloutStrategy := v1alpha1.IncreaseFirstReleaseStrategyType
	if len(c.releasePlan.Strategy) != 0 {
		rolloutStrategy = c.releasePlan.Strategy
	}

	// Determine if we are the first or the second part of the current batch rollout
	// deployment.Replicas == all ReplicaSet.Replicas
	if c.releaseStatus.ObservedWorkloadReplicas == c.getAllReplicas() {
		// we need to finish the first part of the rollout,
		// this may conclude that we've already reached the size (in a rollback case)
		return c.rolloutBatchFirstHalf(ctx, rolloutStrategy)
	}

	// we are at the second half
	targetSize := c.calculateCurrentTarget(c.releaseStatus.ObservedWorkloadReplicas)
	if !c.rolloutBatchSecondHalf(ctx) {
		return false, nil
	}

	// record the finished upgrade action
	klog.V(3).InfoS("upgraded one batch", "current batch", c.releaseStatus.CurrentBatch,
		"target deployment size", targetSize)
	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Rollout", "Finished submitting all upgrade quests for batch %d", c.releaseStatus.CurrentBatch)
	c.releaseStatus.UpgradedReplicas = targetSize
	return true, nil
}

// CheckOneBatchPods checks to see if the pods are all available according to the rollout plan
func (c *DeploymentRolloutController) CheckOneBatchPods(ctx context.Context) (bool, error) {
	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	// get the number of ready pod from target
	readyTargetPodCount := c.targetReplicaSet.Status.ReadyReplicas
	sourcePodCount := getReplicaSetsStatusReplicas(c.sourceReplicaSets...)
	currentBatch := c.releasePlan.Batches[c.releaseStatus.CurrentBatch]
	targetGoal := c.calculateCurrentTarget(c.releaseStatus.ObservedWorkloadReplicas)
	sourceGoal := c.calculateCurrentSource(c.releaseStatus.ObservedWorkloadReplicas)

	// get the rollout strategy
	rolloutStrategy := v1alpha1.IncreaseFirstReleaseStrategyType
	if len(c.releasePlan.Strategy) != 0 {
		rolloutStrategy = c.releasePlan.Strategy
	}
	maxUnavailable := 0
	if currentBatch.MaxUnavailable != nil {
		maxUnavailable, _ = intstr.GetValueFromIntOrPercent(currentBatch.MaxUnavailable, int(c.releaseStatus.ObservedWorkloadReplicas), true)
	}
	klog.InfoS("checking the rolling out progress", "current batch", c.releaseStatus.CurrentBatch,
		"target pod ready count", readyTargetPodCount, "source pod count", sourcePodCount,
		"max unavailable pod allowed", maxUnavailable, "target goal", targetGoal, "source goal", sourceGoal,
		"rolloutStrategy", rolloutStrategy)

	if targetGoal < getReplicaSetsReplicas(c.targetReplicaSet) ||
		sourceGoal > getReplicaSetsReplicas(c.sourceReplicaSets...) ||
		c.releaseStatus.ObservedWorkloadReplicas-c.getAllStatusReadyReplicas() > int32(maxUnavailable) {
		klog.InfoS("the batch is not ready yet", "current batch", c.releaseStatus.CurrentBatch)
		c.releaseStatus.RolloutRetry(fmt.Sprintf(
			"the batch %d is not ready yet with %d target pods ready and %d source pods with %d unavailable allowed",
			c.releaseStatus.CurrentBatch, readyTargetPodCount, sourcePodCount, maxUnavailable))
		return false, nil
	}

	// record the successful upgrade
	c.releaseStatus.UpgradedReadyReplicas = readyTargetPodCount
	klog.InfoS("all pods in current batch are ready", "current batch", c.releaseStatus.CurrentBatch)
	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Available", "Batch %d is available", c.releaseStatus.CurrentBatch)
	return true, nil
}

// FinalizeOneBatch makes sure that the rollout status are updated correctly
func (c *DeploymentRolloutController) FinalizeOneBatch(ctx context.Context) (bool, error) {
	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		// nolint:nilerr
		return false, nil
	}

	targetTarget := getReplicaSetsReplicas(c.targetReplicaSet)
	sourceTarget := getReplicaSetsReplicas(c.sourceReplicaSets...)
	if sourceTarget+targetTarget != c.releaseStatus.ObservedWorkloadReplicas {
		err := fmt.Errorf("deployment targets don't match total rollout, sourceTarget = %d, targetTarget = %d, "+
			"rolloutTargetSize = %d", sourceTarget, targetTarget, c.releaseStatus.ObservedWorkloadReplicas)
		klog.ErrorS(err, "the batch is not valid", "current batch", c.releaseStatus.CurrentBatch)
		return false, err
	}
	return true, nil
}

// Finalize makes sure the Deployment is all upgraded
func (c *DeploymentRolloutController) Finalize(ctx context.Context, succeed bool) bool {
	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		return false
	}

	// release deployment
	if _, err := c.releaseDeployment(ctx, c.deployment); err != nil {
		return false
	}

	c.releaseStatus.StableRevision = c.releaseStatus.UpdateRevision
	c.releaseStatus.LastUpdateRevision = c.releaseStatus.UpdateRevision
	c.releaseStatus.UpdateRevision = ""

	// mark the resource finalized
	//c.rolloutStatus.LastAppliedPodTemplateIdentifier = c.rolloutStatus.NewPodTemplateIdentifier
	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Rollout Finalized", "Rollout resource are finalized, succeed := %t", succeed)
	return true
}

func (c *DeploymentRolloutController) UpdateRevisionChangedDuringRelease(ctx context.Context) (runtime.Object, bool, bool, error) {
	if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
		// don't fail the rollout just because of we can't get the resource
		return nil, false, false, err
	}

	if c.targetReplicaSet.Labels[apps.DefaultDeploymentUniqueLabelKey] != c.releaseStatus.UpdateRevision {
		klog.Warningf("Release Controller Observe UpdateRevision Changed: %v -> %v", c.targetReplicaSet.Labels[apps.DefaultDeploymentUniqueLabelKey], c.releaseStatus.UpdateRevision)
		return c.deployment, true, false, nil
	}

	return c.deployment, false, false, nil
}

func (c *DeploymentRolloutController) MoveToSuitableBatch(ctx context.Context) int32 {
	if c.deployment == nil {
		if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
			return c.releaseStatus.CurrentBatch
		}
	}

	batchCount := len(c.releasePlan.Batches)
	for i := 0; i < batchCount; i++ {
		updateGoal := int32(calculateNewBatchTarget(c.releasePlan, int(c.releaseStatus.ObservedWorkloadReplicas), i))
		stableGoal := c.releaseStatus.ObservedWorkloadReplicas - updateGoal
		updateReplicas := getReplicaSetsReplicas(c.targetReplicaSet)
		stableReplicas := getReplicaSetsReplicas(c.sourceReplicaSets...)
		if updateReplicas < updateGoal || stableReplicas > stableGoal {
			return int32(i)
		}
	}
	return c.releaseStatus.CurrentBatch
}

/* ----------------------------------
The functions below are helper functions
------------------------------------- */
func (c *DeploymentRolloutController) FetchDeploymentAndItsReplicaSets(ctx context.Context) error {
	c.deployment = &apps.Deployment{}
	if err := c.client.Get(context.TODO(), c.targetNamespacedName, c.deployment); err != nil {
		if !apierrors.IsNotFound(err) {
			c.recorder.Event(c.parentController, v1.EventTypeWarning, "GetDeploymentFailed", err.Error())
		}
		return err
	}

	selector, err := metav1.LabelSelectorAsSelector(c.deployment.Spec.Selector)
	if err != nil {
		c.recorder.Event(c.parentController, v1.EventTypeWarning, "ParseDeploymentSelectorFailed", err.Error())
		return err
	}

	rsList := &apps.ReplicaSetList{}
	if err = c.client.List(context.TODO(), rsList, &client.ListOptions{
		Namespace: c.targetNamespacedName.Namespace, LabelSelector: selector}); err != nil {
		c.recorder.Event(c.parentController, v1.EventTypeWarning, "ListReplicaSetFailed", err.Error())
		return err
	}

	var allRSs []*apps.ReplicaSet
	c.targetReplicaSet, c.sourceReplicaSets = nil, nil
	for i := range rsList.Items {
		rs := &rsList.Items[i]
		if ownerRef := metav1.GetControllerOf(rs); ownerRef == nil || ownerRef.UID != c.deployment.GetUID() {
			continue
		}
		allRSs = append(allRSs, rs)
		if deploymentutil.EqualIgnoreHash(&rs.Spec.Template, &c.deployment.Spec.Template) {
			c.targetReplicaSet = rs
		} else {
			c.sourceReplicaSets = append(c.sourceReplicaSets, rs)
		}
	}

	latestReplicaSet, err := c.getNewReplicaSet(c.deployment, allRSs, c.sourceReplicaSets, 0, true)
	if latestReplicaSet == nil {
		c.recorder.Event(c.parentController, v1.EventTypeWarning, "CreateNewReplicaSetFailed", err.Error())
		return err
	} else if err != nil {
		c.recorder.Eventf(c.parentController, v1.EventTypeWarning, "UpdateFailedWhenCreateNewRS", "Get an error when creating the latest replicaset, but the latest replicaset is created successfully, error: %v", err)
		return err
	}

	if c.targetReplicaSet == nil || !deploymentutil.EqualIgnoreHash(&c.targetReplicaSet.Spec.Template, &latestReplicaSet.Spec.Template) {
		klog.V(3).Infof("Create new ReplicaSet for Deployment")
		if c.targetReplicaSet != nil {
			c.sourceReplicaSets = append(c.sourceReplicaSets, c.targetReplicaSet)
		}
		c.targetReplicaSet = latestReplicaSet
	}

	sort.Sort(k8scontroller.ReplicaSetsByCreationTimestamp(c.sourceReplicaSets))
	return nil
}

// calculateRolloutTotalSize fetches the Deployment and returns the replicas (not the actual number of pods)
func (c *DeploymentRolloutController) getDeploymentReplicas() (int32, error) {
	return getDeploymentReplicas(c.deployment), nil
}

func (c *DeploymentRolloutController) GetWorkload() *apps.Deployment {
	return c.deployment
}

// GetUpdateRevision fetches the Deployment and returns the replicas (not the actual number of pods)
func (c *DeploymentRolloutController) GetUpdateRevision() string {
	if c.targetReplicaSet == nil {
		return ""
	}
	return c.targetReplicaSet.Labels[apps.DefaultDeploymentUniqueLabelKey]
}

// check if the replicas in all the rollout batches add up to the right number
func (c *DeploymentRolloutController) verifyRolloutBatchReplicaValue(totalReplicas int32) error {
	// use a common function to check if the sum of all the batches can match the Deployment size
	return verifyBatchesWithRollout(c.releasePlan, totalReplicas)
}

// the target deploy size for the current batch
func (c *DeploymentRolloutController) calculateCurrentTarget(totalSize int32) int32 {
	targetSize := int32(calculateNewBatchTarget(c.releasePlan, int(totalSize), int(c.releaseStatus.CurrentBatch)))
	klog.InfoS("Calculated the number of pods in the target deployment after current batch",
		"current batch", c.releaseStatus.CurrentBatch, "target deploy size", targetSize)
	return targetSize
}

// the source deploy size for the current batch
func (c *DeploymentRolloutController) calculateCurrentSource(totalSize int32) int32 {
	sourceSize := totalSize - c.calculateCurrentTarget(totalSize)
	klog.InfoS("Calculated the number of pods in the source deployment after current batch",
		"current batch", c.releaseStatus.CurrentBatch, "source deploy size", sourceSize)
	return sourceSize
}

func (c *DeploymentRolloutController) checkHealthyAndRecordRevisions() bool {
	activeRSs := k8scontroller.FilterActiveReplicaSets(c.sourceReplicaSets)
	c.releaseStatus.UpdateRevision = c.targetReplicaSet.Labels[apps.DefaultDeploymentUniqueLabelKey]

	switch len(activeRSs) {
	case 0:
		c.releaseStatus.StableRevision = c.targetReplicaSet.Labels[apps.DefaultDeploymentUniqueLabelKey]
		return true
	case 1:
		c.releaseStatus.StableRevision = activeRSs[0].Labels[apps.DefaultDeploymentUniqueLabelKey]
		return true
	}
	if c.releaseStatus.StableRevision == "" {
		return false
	}

	for _, rs := range activeRSs {
		if *rs.Spec.Replicas == 0 {
			continue
		} else if rs.Labels[apps.DefaultDeploymentUniqueLabelKey] == c.releaseStatus.StableRevision {
			return true
		}
	}

	return false
}

func (c *DeploymentRolloutController) rolloutBatchFirstHalf(ctx context.Context,
	rolloutStrategy v1alpha1.ReleaseStrategyType) (finished bool, rolloutError error) {
	targetSize := c.calculateCurrentTarget(c.releaseStatus.ObservedWorkloadReplicas)
	defer func() {
		if finished {
			// record the finished upgrade action
			klog.InfoS("one batch is done already, no need to upgrade", "current batch", c.releaseStatus.CurrentBatch)
			c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Rollout", "upgrade quests for batch %d is already reached, no need to upgrade", c.releaseStatus.CurrentBatch)
			c.releaseStatus.UpgradedReplicas = targetSize
		}
	}()

	if rolloutStrategy == v1alpha1.IncreaseFirstReleaseStrategyType {
		// set the target replica first which should increase its size
		if getReplicaSetsReplicas(c.targetReplicaSet) < targetSize {
			klog.InfoS("set target ReplicaSet replicas", "deploy", c.targetReplicaSet.Name, "targetSize", targetSize)
			targetSize = integer.Int32Min(targetSize, *c.deployment.Spec.Replicas)
			_ = c.scaleReplicaSet(context.TODO(), c.targetReplicaSet, targetSize, c.deployment)
			c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Rollout", "Submitted the increase part of upgrade quests for batch %d, target size = %d", c.releaseStatus.CurrentBatch, targetSize)
			return false, nil
		}

		// do nothing if the target is already reached
		klog.InfoS("target ReplicaSet replicas overshoot the size already", "deploy", c.targetReplicaSet.Name,
			"deployment size", getReplicaSetsReplicas(c.targetReplicaSet), "targetSize", targetSize)
		return true, nil
	}

	if rolloutStrategy == v1alpha1.DecreaseFirstReleaseStrategyType {
		// set the source replicas first which should shrink its size
		sourceSize := c.calculateCurrentSource(c.releaseStatus.ObservedWorkloadReplicas)
		realSourceSize := getReplicaSetsReplicas(c.sourceReplicaSets...)
		if realSourceSize > sourceSize {
			nameToScaleSize := c.computeScaleSizeForReplicaSets()
			// set the source replicas now which should shrink its size
			for _, sourceReplicaSet := range c.sourceReplicaSets {
				scaleSize := nameToScaleSize[sourceReplicaSet.Name]
				klog.InfoS("set source ReplicaSet replicas", "source ReplicaSet", sourceReplicaSet.Name, "sourceSize", scaleSize)
				if err := c.scaleReplicaSet(context.TODO(), sourceReplicaSet, scaleSize, c.deployment); err != nil {
					return false, err
				}
			}
			c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Rollout", "Submitted the decrease part of upgrade quests for batch %d, source size = %d", c.releaseStatus.CurrentBatch, sourceSize)
			return false, nil
		}

		// do nothing if the reduce target is already reached
		klog.InfoS("source ReplicaSets replicas overshoot the size already", "ReplicaSets size", getReplicaSetsReplicas(c.sourceReplicaSets...), "sourceSize", sourceSize)
		return true, nil
	}

	return false, fmt.Errorf("encountered an unknown rolloutStrategy `%s`", rolloutStrategy)
}

func (c *DeploymentRolloutController) rolloutBatchSecondHalf(ctx context.Context) bool {
	sourceSize := c.calculateCurrentSource(c.releaseStatus.ObservedWorkloadReplicas)
	nameToScaleSize := c.computeScaleSizeForReplicaSets()
	allRSs := append(c.sourceReplicaSets, c.targetReplicaSet)
	for _, rs := range allRSs {
		scaleSize := nameToScaleSize[rs.Name]
		klog.InfoS("set source ReplicaSet replicas", "source ReplicaSet", rs.Name, "sourceSize", scaleSize)
		if err := c.scaleReplicaSet(context.TODO(), rs, scaleSize, c.deployment); err != nil {
			return false
		}
	}
	c.recorder.Eventf(c.parentController, v1.EventTypeNormal, "Batch Rollout", "Submitted the decrease part of upgrade quests for batch %d, source size = %d", c.releaseStatus.CurrentBatch, sourceSize)
	return true
}

func (c *DeploymentRolloutController) computeScaleSizeForReplicaSets() map[string]int32 {
	allRSs := k8scontroller.FilterActiveReplicaSets(append(c.sourceReplicaSets, c.targetReplicaSet))
	allRSsReplicas := deploymentutil.GetReplicaCountForReplicaSets(allRSs)

	allowedSize := int32(0)
	if *(c.deployment.Spec.Replicas) > 0 {
		allowedSize = *(c.deployment.Spec.Replicas)
	}

	// Number of additional replicas that can be either added or removed from the total
	// replicas count. These replicas should be distributed proportionally to the active
	// replica sets.
	deploymentReplicasToAdd := allowedSize - allRSsReplicas
	// make sure the canary target replicas size firstly.
	targetSize := c.calculateCurrentTarget(c.releaseStatus.ObservedWorkloadReplicas)
	targetSize = integer.Int32Min(targetSize, *c.deployment.Spec.Replicas)
	deploymentReplicasToAdd = deploymentReplicasToAdd - (targetSize - *c.targetReplicaSet.Spec.Replicas)

	// Iterate over all active replica sets and estimate proportions for each of them.
	// The absolute value of deploymentReplicasAdded should never exceed the absolute
	// value of deploymentReplicasToAdd.
	deploymentReplicasAdded := int32(0)
	nameToSize := make(map[string]int32)
	nameToSize[c.targetReplicaSet.Name] = targetSize

	// The additional replicas should be distributed proportionally amongst the active
	// replica sets from the larger to the smaller in size replica set. Scaling direction
	// drives what happens in case we are trying to scale replica sets of the same size.
	// In such a case when scaling up, we should scale up newer replica sets first, and
	// when scaling down, we should scale down older replica sets first.
	oldRSs := c.sourceReplicaSets
	switch {
	case deploymentReplicasToAdd > 0:
		sort.Sort(k8scontroller.ReplicaSetsBySizeNewer(oldRSs))
	case deploymentReplicasToAdd < 0:
		sort.Sort(k8scontroller.ReplicaSetsBySizeOlder(oldRSs))
	}

	// scale the old replicas in proportion
	expectedOldReplicas := *c.deployment.Spec.Replicas - targetSize
	for i := range oldRSs {
		rs := oldRSs[i]
		// Estimate proportions if we have replicas to add, otherwise simply populate
		// nameToSize with the current sizes for each replica set.
		if deploymentReplicasToAdd != 0 {
			proportion := deploymentutil.GetProportion(rs, *c.deployment, deploymentReplicasToAdd, deploymentReplicasAdded, expectedOldReplicas)

			nameToSize[rs.Name] = *(rs.Spec.Replicas) + proportion
			deploymentReplicasAdded += proportion
		} else {
			nameToSize[rs.Name] = *(rs.Spec.Replicas)
		}
	}

	// deal with the rest budget for old replicas
	leftover := deploymentReplicasToAdd - deploymentReplicasAdded
	for _, rs := range oldRSs {
		leftoverAdded := int32(0)
		if leftover > 0 {
			leftoverAdded = leftover
		} else if leftover < 0 {
			leftoverAdded = integer.Int32Max(-*rs.Spec.Replicas, leftover)
		}
		leftover -= leftoverAdded
		nameToSize[rs.Name] += leftoverAdded
	}

	if leftover != 0 {
		nameToSize[c.targetReplicaSet.Name] += leftover
	}

	for _, rs := range allRSs {
		if nameToSize[rs.Name] < 0 {
			nameToSize[rs.Name] = 0
		}
	}

	return nameToSize
}

func (c *DeploymentRolloutController) getAllReplicas() int32 {
	return getReplicaSetsReplicas(c.targetReplicaSet) + getReplicaSetsReplicas(c.sourceReplicaSets...)
}

func (c *DeploymentRolloutController) getAllStatusReplicas() int32 {
	return getReplicaSetsStatusReplicas(c.targetReplicaSet) + getReplicaSetsStatusReplicas(c.sourceReplicaSets...)
}

func (c *DeploymentRolloutController) getAllStatusReadyReplicas() int32 {
	return getReplicaSetsStatusReadyReplicas(c.targetReplicaSet) + getReplicaSetsStatusReadyReplicas(c.sourceReplicaSets...)
}

func (c *DeploymentRolloutController) ReplicasChangedDuringRelease(ctx context.Context) (bool, error) {
	if c.releaseStatus.ObservedWorkloadReplicas == -1 {
		return false, nil
	}
	if c.releaseStatus.ReleasingState == v1alpha1.VerifyingSpecState &&
		c.releaseStatus.UpdateRevision == "" {
		return false, nil
	}

	if c.deployment == nil {
		if err := c.FetchDeploymentAndItsReplicaSets(ctx); err != nil {
			return false, err
		}
	}
	if *c.deployment.Spec.Replicas == c.releaseStatus.ObservedWorkloadReplicas {
		return false, nil
	}
	return true, nil
}

func getReplicaSetsReplicas(replicaSets ...*apps.ReplicaSet) int32 {
	var replicas int32
	for _, rs := range replicaSets {
		if rs != nil && rs.Spec.Replicas != nil {
			replicas += *rs.Spec.Replicas
		}
	}
	return replicas
}

func getReplicaSetsStatusReplicas(replicaSets ...*apps.ReplicaSet) int32 {
	var replicas int32
	for _, rs := range replicaSets {
		if rs != nil {
			replicas += rs.Status.Replicas
		}
	}
	return replicas
}

func getReplicaSetsStatusReadyReplicas(replicaSets ...*apps.ReplicaSet) int32 {
	var replicas int32
	for _, rs := range replicaSets {
		if rs != nil {
			replicas += rs.Status.ReadyReplicas
		}
	}
	return replicas
}
