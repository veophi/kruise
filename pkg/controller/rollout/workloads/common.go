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
	"fmt"
	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

// verifyBatchesWithRollout verifies that the the sum of all the batch replicas is valid given the total replica
// each batch replica can be absolute or a percentage
func verifyBatchesWithRollout(rolloutSpec *v1alpha1.ReleasePlan, totalReplicas int32) error {
	// If rolloutBatches length equal to zero will cause index out of bounds panic, guarantee don't crash whole vela controller
	if len(rolloutSpec.Batches) == 0 {
		return fmt.Errorf("the rolloutPlan must have batches")
	}
	// if not set, the sum of all the batch sizes minus the last batch cannot be more than the totalReplicas
	totalRollout := 0
	for i := 0; i < len(rolloutSpec.Batches)-1; i++ {
		rb := rolloutSpec.Batches[i]
		batchSize, _ := intstr.GetValueFromIntOrPercent(&rb.Replicas, int(totalReplicas), true)
		totalRollout += batchSize
	}
	if totalRollout >= int(totalReplicas) {
		return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, totalReplicas size = %d",
			totalRollout, totalReplicas)
	}

	// include the last batch if it has an int value
	// we ignore the last batch percentage since it is very likely to cause rounding errors
	lastBatch := rolloutSpec.Batches[len(rolloutSpec.Batches)-1]
	if lastBatch.Replicas.Type == intstr.Int {
		totalRollout += int(lastBatch.Replicas.IntVal)
		// now that they should be the same
		if totalRollout != int(totalReplicas) {
			return fmt.Errorf("the rollout plan batch size mismatch, total batch size = %d, totalReplicas size = %d",
				totalRollout, totalReplicas)
		}
	}
	return nil
}

func calculateNewBatchTarget(rolloutSpec *v1alpha1.ReleasePlan, workloadReplicas, currentBatch int) int {
	if currentBatch == len(rolloutSpec.Batches)-1 {
		// special handle the last batch, we ignore the rest of the batch in case there are rounding errors
		klog.V(3).InfoS("use the target size as the total pod target for the last rolling batch",
			"current batch", currentBatch, "new pod target", workloadReplicas)
		return workloadReplicas
	}

	batchSize, _ := intstr.GetValueFromIntOrPercent(&rolloutSpec.Batches[currentBatch].Replicas, workloadReplicas, true)
	if batchSize > workloadReplicas {
		klog.Warning("releasePlan has wrong batch replicas, batches[%d].replicas %v is more than workload.replicas %v", currentBatch, batchSize, workloadReplicas)
		batchSize = workloadReplicas
	}

	klog.V(3).InfoS("calculated the number of new pod size", "current batch", currentBatch,
		"new pod target", batchSize)
	return batchSize
}

func getDeploymentReplicas(deploy *apps.Deployment) int32 {
	// replicas default is 0
	if deploy.Spec.Replicas != nil {
		return *deploy.Spec.Replicas
	}
	return 0
}

func getCloneSetReplicas(clone *v1alpha1.CloneSet) int32 {
	// replicas default is 0
	if clone.Spec.Replicas != nil {
		return *clone.Spec.Replicas
	}
	return 0
}
