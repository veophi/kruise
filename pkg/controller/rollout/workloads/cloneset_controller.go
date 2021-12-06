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
	"encoding/json"
	"fmt"
	"github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ByBatchReleaseControlAnnotation = "batch-release-control"
const StashCloneSetPartition = "stash-cloneset-updateStrategy-partition"

// cloneSetController is the place to hold fields needed for handle Deployment type of workloads
type cloneSetController struct {
	workloadController
	targetNamespacedName types.NamespacedName
}

// add the parent controller to the owner of the deployment, unpause it and initialize the size
// before kicking start the update and start from every pod in the old version
func (c *cloneSetController) claimCloneSet(ctx context.Context, clone *v1alpha1.CloneSet) (bool, error) {
	releasePatch := client.MergeFrom(c.parentController)
	refByte, _ := json.Marshal(metav1.NewControllerRef(clone, clone.GetObjectKind().GroupVersionKind()))
	if c.parentController.Annotations == nil {
		c.parentController.Annotations = make(map[string]string)
	}
	c.parentController.Annotations[ByBatchReleaseControlAnnotation] = string(refByte)
	if err := c.client.Patch(context.TODO(), c.parentController, releasePatch); err != nil {
		c.recorder.Eventf(c.parentController, v1.EventTypeWarning, "ReleasePatchFailed", err.Error())
		c.releaseStatus.RolloutRetry(err.Error())
		return false, err
	}

	clonePatch := client.MergeFrom(clone.DeepCopy())
	if clone.Spec.UpdateStrategy.Partition != nil {
		by, _ := json.Marshal(clone.Spec.UpdateStrategy.Partition)
		clone.Annotations[StashCloneSetPartition] = string(by)
	}
	clone.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.String, StrVal: "100%"}
	clone.Spec.UpdateStrategy.Paused = false

	if err := c.client.Patch(context.TODO(), clone, clonePatch); err != nil {
		c.recorder.Eventf(c.parentController, v1.EventTypeWarning, "ClaimCloneSetFailed", err.Error())
		c.releaseStatus.RolloutRetry(err.Error())
		return false, err
	}

	klog.V(3).Info("Claim CloneSet Successfully")
	return false, nil
}

// remove the parent controller from the deployment's owner list
func (c *cloneSetController) releaseCloneSet(ctx context.Context, clone *v1alpha1.CloneSet) (bool, error) {
	var found bool
	var refByte string
	if refByte, found = c.parentController.Annotations[ByBatchReleaseControlAnnotation]; found && refByte != "" {
		ref := &metav1.OwnerReference{}
		if err := json.Unmarshal([]byte(refByte), ref); err != nil {
			found = false
			klog.Error("failed to decode controller annotations of BatchRelease")
		} else if ref.UID != clone.UID {
			found = false
		}
	}

	if !found {
		klog.V(3).InfoS("the cloneset is already released", "CloneSet", clone.Name)
		return true, nil
	}

	patch := fmt.Sprintf(`{"metadata": {"annotations": {"%v": null}}}`, ByBatchReleaseControlAnnotation)
	if err := c.client.Patch(context.TODO(), c.parentController, client.RawPatch(types.MergePatchType, []byte(patch))); err != nil {
		c.recorder.Eventf(c.parentController, v1.EventTypeWarning, "ReleaseControllerFailed", err.Error())
		c.releaseStatus.RolloutRetry(err.Error())
		return false, err
	}

	clonePatch := client.MergeFrom(clone.DeepCopy())
	if by, ok := clone.Annotations[StashCloneSetPartition]; ok {
		partition := &intstr.IntOrString{}
		if err := json.Unmarshal([]byte(by), partition); err != nil {
			klog.Error("failed to decode partition annotations for cloneset")
		} else {
			clone.Spec.UpdateStrategy.Partition = partition
			delete(clone.Annotations, StashCloneSetPartition)
		}
	}

	clone.Spec.UpdateStrategy.Paused = true
	if err := c.client.Patch(context.TODO(), clone, clonePatch); err != nil {
		c.recorder.Eventf(c.parentController, v1.EventTypeWarning, "ReleaseCloneSetFailed", err.Error())
		c.releaseStatus.RolloutRetry(err.Error())
		return false, err
	}

	klog.V(3).Info("Release CloneSet Successfully")
	return false, nil
}

// scale the deployment
func (c *cloneSetController) patchCloneSetPartition(ctx context.Context, clone *v1alpha1.CloneSet, partition int32) error {
	clonePatch := client.MergeFrom(clone.DeepCopy())
	clone.Spec.UpdateStrategy.Partition = &intstr.IntOrString{Type: intstr.Int, IntVal: partition}

	if err := c.client.Patch(context.TODO(), clone, clonePatch); err != nil {
		c.recorder.Eventf(c.parentController, v1.EventTypeWarning, "PatchPartitionFailed", "Failed to update the CloneSet to the correct target partition %d, error: %v", partition, err)
		c.releaseStatus.RolloutRetry(err.Error())
		return err
	}

	klog.InfoS("Submitted modified partition quest for cloneset", "CloneSet",
		clone.GetName(), "target partition size", partition, "batch", c.releaseStatus.CurrentBatch)
	return nil
}
