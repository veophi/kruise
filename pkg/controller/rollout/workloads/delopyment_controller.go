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

	deploymentutil "github.com/openkruise/kruise/pkg/controller/rollout/workloads/copy_from_deployment/util"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StashDeploymentStrategyType   = "stash-strategy-type"
	StashDeploymentMaxUnavailable = "stash-maxUnavailable"
	StashDeploymentMaxSurge       = "stash-maxSurge"
)

// deploymentController is the place to hold fields needed for handle Deployment type of workloads
type deploymentController struct {
	workloadController
	targetNamespacedName types.NamespacedName
}

// add the parent controller to the owner of the deployment, unpause it and initialize the size
// before kicking start the update and start from every pod in the old version
func (c *deploymentController) claimDeployment(ctx context.Context, deploy *apps.Deployment, initSize *int32) (bool, error) {
	if controller := metav1.GetControllerOf(deploy); controller != nil {
		// it's already there
		return true, nil
	}

	deployPatch := client.MergeFrom(deploy.DeepCopy())

	// add the parent controller to the owner of the deployment
	ref := metav1.NewControllerRef(c.parentController, c.parentController.GetObjectKind().GroupVersionKind())
	deploy.SetOwnerReferences(append(deploy.GetOwnerReferences(), *ref))

	deploy.Annotations[StashDeploymentStrategyType] = string(deploy.Spec.Strategy.Type)
	if deploy.Spec.Strategy.RollingUpdate != nil {
		if deploy.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			by, _ := json.Marshal(deploy.Spec.Strategy.RollingUpdate.MaxSurge)
			deploy.Annotations[StashDeploymentMaxSurge] = string(by)
		}
		if deploy.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			by, _ := json.Marshal(deploy.Spec.Strategy.RollingUpdate.MaxUnavailable)
			deploy.Annotations[StashDeploymentMaxUnavailable] = string(by)
		}
	}

	deploy.Spec.Paused = true
	deploy.Spec.Strategy.Type = apps.RecreateDeploymentStrategyType
	deploy.Spec.Strategy.RollingUpdate = nil

	// patch the Deployment
	if err := c.client.Patch(ctx, deploy, deployPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Eventf(deploy, v1.EventTypeWarning, "DeploymentUpdateError", err.Error())
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, err
	}
	return false, nil
}

// scale the deployment
func (c *deploymentController) scaleDeployment(ctx context.Context, deploy *apps.Deployment, size int32) error {
	deployPatch := client.MergeFrom(deploy.DeepCopy())
	deploy.Spec.Replicas = pointer.Int32Ptr(size)

	// patch the Deployment
	if err := c.client.Patch(ctx, deploy, deployPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Eventf(deploy, v1.EventTypeWarning, "DeploymentScale", "Failed to update the deployment %s to the correct target %d, error: %v", deploy.GetName(), size, err)
		c.rolloutStatus.RolloutRetry(err.Error())
		return err
	}

	klog.InfoS("Submitted upgrade quest for deployment", "deployment",
		deploy.GetName(), "target replica size", size, "batch", c.rolloutStatus.CurrentBatch)
	return nil
}

func (c *deploymentController) scaleReplicaSet(ctx context.Context, rs *apps.ReplicaSet, newScale int32, deployment *apps.Deployment) error {
	var scalingOperation string
	switch {
	case *(rs.Spec.Replicas) < newScale:
		scalingOperation = "up"
	case *(rs.Spec.Replicas) > newScale:
		scalingOperation = "down"
	}

	sizeNeedsUpdate := *(rs.Spec.Replicas) != newScale
	annotationsNeedUpdate := deploymentutil.ReplicasAnnotationsNeedUpdate(rs, *(deployment.Spec.Replicas), *(deployment.Spec.Replicas)+deploymentutil.MaxSurge(*deployment))

	var err error
	if sizeNeedsUpdate || annotationsNeedUpdate {
		rsCopy := rs.DeepCopy()
		*(rsCopy.Spec.Replicas) = newScale
		deploymentutil.SetReplicasAnnotations(rsCopy, *(deployment.Spec.Replicas), *(deployment.Spec.Replicas)+deploymentutil.MaxSurge(*deployment))
		err = c.client.Update(ctx, rsCopy, client.FieldOwner(deployment.GetUID()))
		if err == nil && sizeNeedsUpdate {
			c.recorder.Eventf(deployment, v1.EventTypeNormal, "ScalingReplicaSet", "Scaled %s replica set %s to %d", scalingOperation, rs.Name, newScale)
		}
	}
	return err
}

// remove the parent controller from the deployment's owner list
func (c *deploymentController) releaseDeployment(ctx context.Context, deploy *apps.Deployment) (bool, error) {
	deployPatch := client.MergeFrom(deploy.DeepCopy())

	var newOwnerList []metav1.OwnerReference
	found := false
	for _, owner := range deploy.GetOwnerReferences() {
		if owner.Kind == c.parentController.GetObjectKind().GroupVersionKind().Kind &&
			owner.APIVersion == c.parentController.GetObjectKind().GroupVersionKind().GroupVersion().String() {
			found = true
			continue
		}
		newOwnerList = append(newOwnerList, owner)
	}
	if !found {
		klog.InfoS("the deployment is already released", "deploy", deploy.Name)
		return true, nil
	}
	deploy.SetOwnerReferences(newOwnerList)

	var maxSurge, maxUnavailable *intstr.IntOrString
	if by, ok := deploy.Annotations[StashDeploymentMaxSurge]; ok {
		maxSurge = &intstr.IntOrString{}
		if err := json.Unmarshal([]byte(by), maxSurge); err != nil {
			klog.Error("failed to decode maxSurge annotations")
		}
	}
	if by, ok := deploy.Annotations[StashDeploymentMaxUnavailable]; ok {
		maxUnavailable = &intstr.IntOrString{}
		if err := json.Unmarshal([]byte(by), maxUnavailable); err != nil {
			klog.Error("failed to decode maxUnavailable annotations")
		}
	}

	stashType := apps.DeploymentStrategyType(deploy.Annotations[StashDeploymentStrategyType])
	switch stashType {
	case apps.RollingUpdateDeploymentStrategyType:
		deploy.Spec.Strategy.RollingUpdate = &apps.RollingUpdateDeployment{
			MaxSurge:       maxSurge,
			MaxUnavailable: maxUnavailable,
		}
		fallthrough
	case apps.RecreateDeploymentStrategyType:
		deploy.Spec.Strategy.Type = stashType
	default:
		klog.Error("unknown strategy type gotten from annotations")
	}
	deploy.Spec.Paused = true

	// patch the Deployment
	if err := c.client.Patch(ctx, deploy, deployPatch, client.FieldOwner(c.parentController.GetUID())); err != nil {
		c.recorder.Eventf(deploy, v1.EventTypeWarning, "ReleaseDeploymentFailed", err.Error())
		c.rolloutStatus.RolloutRetry(err.Error())
		return false, err
	}
	return false, nil
}
