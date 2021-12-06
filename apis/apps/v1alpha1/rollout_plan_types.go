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

package v1alpha1

import (
	"github.com/openkruise/kruise/apis/apps/v1alpha1/condition"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ReleaseStrategyType defines strategies for pods rollout
type ReleaseStrategyType string

const (
	// IncreaseFirstReleaseStrategyType indicates that we increase the target resources first
	IncreaseFirstReleaseStrategyType ReleaseStrategyType = "IncreaseFirst"

	// DecreaseFirstReleaseStrategyType indicates that we decrease the source resources first
	DecreaseFirstReleaseStrategyType ReleaseStrategyType = "DecreaseFirst"
)

// RollingState is the overall rollout state
type RollingState string

const (
	// LocatingTargetAppState indicates that the rollout is in the stage of locating target app
	// we use this state to make sure we special handle the target app successfully only once
	LocatingTargetAppState RollingState = "locatingTargetApp"
	// VerifyingSpecState indicates that the rollout is in the stage of verifying the rollout settings
	// and the controller can locate both the target and the source
	VerifyingSpecState RollingState = "verifyingSpec"
	// InitializingState indicates that the rollout is initializing all the new resources
	InitializingState RollingState = "initializing"
	// RollingInBatchesState indicates that the rollout starts rolling
	RollingInBatchesState RollingState = "rollingInBatches"
	// FinalisingState indicates that the rollout is finalizing, possibly clean up the old resources, adjust traffic
	FinalisingState RollingState = "finalising"
	// RolloutFailingState indicates that the rollout is failing
	// one needs to finalize it before mark it as failed by cleaning up the old resources, adjust traffic
	RolloutFailingState RollingState = "rolloutFailing"
	// RolloutSucceedState indicates that rollout successfully completed to match the desired target state
	RolloutSucceedState RollingState = "rolloutSucceed"
	// RolloutAbandoningState indicates that the rollout is being abandoned
	// we need to finalize it by cleaning up the old resources, adjust traffic and return control back to its owner
	RolloutAbandoningState RollingState = "rolloutAbandoning"
	// RolloutDeletingState indicates that the rollout is being deleted
	// we need to finalize it by cleaning up the old resources, adjust traffic and return control back to its owner
	RolloutDeletingState RollingState = "RolloutDeletingState"
	// RolloutFailedState indicates that rollout is failed, the target replica is not reached
	// we can not move forward anymore, we will let the client to decide when or whether to revert.
	RolloutFailedState RollingState = "rolloutFailed"
)

// BatchRollingState is the sub state when the rollout is on the fly
type BatchRollingState string

const (
	// BatchInitializingState still rolling the batch, the batch rolling is not completed yet
	BatchInitializingState BatchRollingState = "batchInitializing"
	// BatchInRollingState still rolling the batch, the batch rolling is not completed yet
	BatchInRollingState BatchRollingState = "batchInRolling"
	// BatchVerifyingState verifying if the application is ready to roll.
	BatchVerifyingState BatchRollingState = "batchVerifying"
	// BatchRolloutFailedState indicates that the batch didn't get the manual or automatic approval
	BatchRolloutFailedState BatchRollingState = "batchVerifyFailed"
	// BatchFinalizingState indicates that all the pods in the are available, we can move on to the next batch
	BatchFinalizingState BatchRollingState = "batchFinalizing"
	// BatchReadyState indicates that all the pods in the are upgraded and its state is ready
	BatchReadyState BatchRollingState = "batchReady"
)

// ReleasePlan fines the details of the rollout plan
type ReleasePlan struct {

	// RolloutStrategy defines strategies for the rollout plan
	// The default is IncreaseFirstRolloutStrategyType
	// +optional
	Strategy ReleaseStrategyType `json:"strategy,omitempty"`

	// The exact distribution among batches.
	// its size has to be exactly the same as the NumBatches (if set)
	// The total number cannot exceed the targetSize or the size of the source resource
	// We will IGNORE the last batch's replica field if it's a percentage since round errors can lead to inaccurate sum
	// We highly recommend to leave the last batch's replica field empty
	// +optional
	Batches []ReleaseBatch `json:"batches,omitempty"`

	// All pods in the batches up to the batchPartition (included) will have
	// the target resource specification while the rest still have the source resource
	// This is designed for the operators to manually rollout
	// Default is the the number of batches which will rollout all the batches
	// +optional
	BatchPartition *int32 `json:"batchPartition,omitempty"`

	// Paused the rollout, default is false
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// ReleaseBatch is used to describe how the each batch rollout should be
type ReleaseBatch struct {
	// Replicas is the number of pods to upgrade in this batch
	// it can be an absolute number (ex: 5) or a percentage of total pods
	// we will ignore the percentage of the last batch to just fill the gap
	// +optional
	// it is mutually exclusive with the PodList field
	Replicas intstr.IntOrString `json:"replicas,omitempty"`

	// MaxUnavailable is the max allowed number of pods that is unavailable
	// during the upgrade. We will mark the batch as ready as long as there are less
	// or equal number of pods unavailable than this number.
	// default = 0
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`

	// The wait time, in seconds, between instances upgrades, default = 0
	// +optional
	PauseSeconds int64 `json:"pauseSeconds,omitempty"`
}

// BatchReleaseStatus defines the observed state of a rollout plan
type BatchReleaseStatus struct {
	// Conditions represents the latest available observations of a CloneSet's current state.
	condition.ConditionedStatus `json:",inline"`

	// ReleasingState is the Rollout State
	ReleasingState RollingState `json:"releasingState"`

	// ReleasingBatchState only meaningful when the Status is rolling
	// +optional
	ReleasingBatchState BatchRollingState `json:"releasingBatchState"`

	// ObservedWorkloadReplicas is the size of the target resources. This is determined once the initial spec verification
	// and does not change until the rollout is restarted
	ObservedWorkloadReplicas int32 `json:"observedWorkloadReplicas,omitempty"`

	ObservedReleasePlanHash string `json:"observedReleasePlanHash,omitempty"`

	// StableRevision
	StableRevision string `json:"stableRevision,omitempty"`

	// UpdateRevision
	UpdateRevision string `json:"updateRevision,omitempty"`

	// LastUpdateRevision is a string that uniquely represent the last pod template
	// each workload type could use different ways to identify that so we cannot compare between resources
	// We update this field only after a successful rollout
	LastUpdateRevision string `json:"lastUpdateRevision,omitempty"`

	LastBatchFinalizedTime metav1.Time `json:"lastBatchFinalizedTime,omitempty"`

	// The current batch the rollout is working on/blocked
	// it starts from 0
	CurrentBatch int32 `json:"currentBatch"`

	// UpgradedReplicas is the number of Pods upgraded by the rollout controller
	UpgradedReplicas int32 `json:"upgradedReplicas"`

	// UpgradedReadyReplicas is the number of Pods upgraded by the rollout controller that have a Ready Condition.
	UpgradedReadyReplicas int32 `json:"upgradedReadyReplicas"`
}
