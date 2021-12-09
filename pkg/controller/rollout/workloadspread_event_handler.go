/*
Copyright 2021 The Kruise Authors.

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
	"github.com/openkruise/kruise/pkg/controller/rollout/workloads"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
)

type EventAction string

const (
	CreateEventAction EventAction = "Create"
	DeleteEventAction EventAction = "Delete"
)

var (
	controllerKruiseKindWS = appsalphav1.SchemeGroupVersion.WithKind("WorkloadSpread")
	controllerKruiseKindCS = appsalphav1.SchemeGroupVersion.WithKind("CloneSet")
	controllerKindRS       = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	controllerKindDep      = appsv1.SchemeGroupVersion.WithKind("Deployment")
	controllerKindJob      = batchv1.SchemeGroupVersion.WithKind("Job")
)

var _ handler.EventHandler = &workloadEventHandler{}

type workloadEventHandler struct {
	client.Reader
}

func (w workloadEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	w.handleWorkload(q, evt.Object, CreateEventAction)
}

func (w workloadEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	var gvk schema.GroupVersionKind
	var oldAccessor metav1.Object
	var newAccessor metav1.Object

	var paused bool
	var statusReplicas int32
	var updateReplicas int32
	var workloadReplicas int32
	switch evt.ObjectNew.(type) {
	case *appsalphav1.CloneSet:
		oldAccessor = &evt.ObjectOld.(*appsalphav1.CloneSet).ObjectMeta
		newAccessor = &evt.ObjectNew.(*appsalphav1.CloneSet).ObjectMeta
		statusReplicas = evt.ObjectNew.(*appsalphav1.CloneSet).Status.Replicas
		updateReplicas = evt.ObjectNew.(*appsalphav1.CloneSet).Status.UpdatedReplicas
		workloadReplicas = *evt.ObjectNew.(*appsalphav1.CloneSet).Spec.Replicas
		paused = evt.ObjectNew.(*appsalphav1.CloneSet).Spec.UpdateStrategy.Paused
		gvk = controllerKruiseKindCS
	case *appsv1.Deployment:
		oldAccessor = &evt.ObjectOld.(*appsv1.Deployment).ObjectMeta
		newAccessor = &evt.ObjectNew.(*appsv1.Deployment).ObjectMeta
		statusReplicas = evt.ObjectNew.(*appsv1.Deployment).Status.Replicas
		updateReplicas = evt.ObjectNew.(*appsv1.Deployment).Status.UpdatedReplicas
		workloadReplicas = *evt.ObjectNew.(*appsv1.Deployment).Spec.Replicas
		paused = evt.ObjectNew.(*appsv1.Deployment).Spec.Paused
		gvk = controllerKindDep
	default:
		return
	}
	if !paused {
		return
	}

	_, controlled := newAccessor.GetAnnotations()[workloads.BatchReleaseControlAnnotation]
	if oldAccessor.GetGeneration() != newAccessor.GetGeneration() ||
		(paused && controlled && workloadReplicas == statusReplicas && updateReplicas != workloadReplicas) { // watch for scale event, reconcile when scale done
		workloadNsn := types.NamespacedName{
			Namespace: evt.ObjectNew.GetNamespace(),
			Name:      evt.ObjectNew.GetName(),
		}
		rs, err := w.getBatchRelease(workloadNsn, gvk)
		if err != nil {
			klog.Errorf("unable to get BatchRelease related with %s (%s/%s), err: %v",
				gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, err)
			return
		}
		if rs != nil {
			klog.V(3).Infof("%s (%s/%s) changed generation from %d to %d managed by BatchRelease (%s/%s)",
				gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, oldAccessor.GetGeneration(), newAccessor.GetGeneration(), rs.GetNamespace(), rs.GetName())
			nsn := types.NamespacedName{Namespace: rs.GetNamespace(), Name: rs.GetName()}
			q.Add(reconcile.Request{NamespacedName: nsn})
		}
	}
}

func (w workloadEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	w.handleWorkload(q, evt.Object, DeleteEventAction)
}

func (w workloadEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (w *workloadEventHandler) handleWorkload(q workqueue.RateLimitingInterface,
	obj client.Object, action EventAction) {
	var gvk schema.GroupVersionKind
	switch obj.(type) {
	case *appsalphav1.CloneSet:
		gvk = controllerKruiseKindCS
	case *appsv1.Deployment:
		gvk = controllerKindDep
	default:
		return
	}

	workloadNsn := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ws, err := w.getBatchRelease(workloadNsn, gvk)
	if err != nil {
		klog.Errorf("unable to get WorkloadSpread related with %s (%s/%s), err: %v",
			gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, err)
		return
	}
	if ws != nil {
		klog.V(5).Infof("%s %s (%s/%s) and reconcile WorkloadSpread (%s/%s)",
			action, gvk.Kind, workloadNsn.Namespace, workloadNsn.Namespace, ws.Namespace, ws.Name)
		nsn := types.NamespacedName{Namespace: ws.GetNamespace(), Name: ws.GetName()}
		q.Add(reconcile.Request{NamespacedName: nsn})
	}
}

func (w *workloadEventHandler) getBatchRelease(
	workloadNamespaceName types.NamespacedName,
	gvk schema.GroupVersionKind) (*appsalphav1.BatchRelease, error) {
	bsList := &appsalphav1.BatchReleaseList{}
	listOptions := &client.ListOptions{Namespace: workloadNamespaceName.Namespace}
	if err := w.List(context.TODO(), bsList, listOptions); err != nil {
		klog.Errorf("List WorkloadSpread failed: %s", err.Error())
		return nil, err
	}

	for _, bs := range bsList.Items {
		if bs.DeletionTimestamp != nil {
			continue
		}

		targetRef := bs.Spec.TargetRef
		targetGV, err := schema.ParseGroupVersion(targetRef.APIVersion)
		if err != nil {
			klog.Errorf("failed to parse targetRef's group version: %s", targetRef.APIVersion)
			continue
		}

		if targetRef.Kind == gvk.Kind && targetGV.Group == gvk.Group && targetRef.Name == workloadNamespaceName.Name {
			return &bs, nil
		}
	}

	return nil, nil
}
