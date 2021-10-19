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

package workloadspread

import (
	"context"
	"encoding/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsalphav1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	wsutil "github.com/openkruise/kruise/pkg/util/workloadspread"
)

type EventAction string

const (
	CreateEventAction EventAction = "Create"
	UpdateEventAction EventAction = "Update"
	DeleteEventAction EventAction = "Delete"
)

var _ handler.EventHandler = &podEventHandler{}

type podEventHandler struct{}

func (p *podEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.handlePod(q, evt.Object, CreateEventAction)
}

func (p *podEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	oldPod := evt.ObjectOld.(*corev1.Pod)
	newPod := evt.ObjectNew.(*corev1.Pod)

	if kubecontroller.IsPodActive(oldPod) && !kubecontroller.IsPodActive(newPod) {
		p.handlePod(q, newPod, UpdateEventAction)
	}
}

func (p *podEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	p.handlePod(q, evt.Object, DeleteEventAction)
}

func (p *podEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {}

func (p *podEventHandler) handlePod(q workqueue.RateLimitingInterface, obj runtime.Object, action EventAction) {
	pod := obj.(*corev1.Pod)
	if value, exist := pod.GetAnnotations()[wsutil.MatchedWorkloadSpreadSubsetAnnotations]; exist {
		injectWorkloadSpread := &wsutil.InjectWorkloadSpread{}
		if err := json.Unmarshal([]byte(value), injectWorkloadSpread); err != nil {
			klog.Errorf("Failed to unmarshal %s to WorkloadSpread", value)
			return
		}
		nsn := types.NamespacedName{Namespace: pod.GetNamespace(), Name: injectWorkloadSpread.Name}
		klog.V(5).Infof("%s Pod (%s/%s) and reconcile WorkloadSpread (%s/%s)",
			action, pod.Namespace, pod.Name, nsn.Namespace, nsn.Name)
		q.Add(reconcile.Request{NamespacedName: nsn})
	}
}

var _ handler.EventHandler = &workloadEventHandler{}

type workloadEventHandler struct {
	client.Reader
}

func (w workloadEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	w.handleWorkload(q, evt.Object, CreateEventAction)
}

func (w workloadEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	var gvk schema.GroupVersionKind
	var oldGeneration int64
	var newGeneration int64
	var workload metav1.Object

	switch evt.ObjectNew.(type) {
	case *appsalphav1.CloneSet:
		oldGeneration = evt.ObjectOld.(*appsalphav1.CloneSet).Generation
		newGeneration = evt.ObjectNew.(*appsalphav1.CloneSet).Generation
		workload = evt.ObjectNew.(*appsalphav1.CloneSet)
		gvk = controllerKruiseKindCS
	case *appsv1.Deployment:
		oldGeneration = evt.ObjectOld.(*appsv1.Deployment).Generation
		newGeneration = evt.ObjectNew.(*appsv1.Deployment).Generation
		workload = evt.ObjectNew.(*appsv1.Deployment)
		gvk = controllerKindDep
	case *appsv1.ReplicaSet:
		oldGeneration = evt.ObjectOld.(*appsv1.ReplicaSet).Generation
		newGeneration = evt.ObjectNew.(*appsv1.ReplicaSet).Generation
		workload = evt.ObjectNew.(*appsv1.ReplicaSet)
		gvk = controllerKindRS
	case *batchv1.Job:
		oldGeneration = evt.ObjectOld.(*batchv1.Job).Generation
		newGeneration = evt.ObjectNew.(*batchv1.Job).Generation
		workload = evt.ObjectNew.(*batchv1.Job)
		gvk = controllerKindJob
	default:
		return
	}

	// workload replicas changed, and reconcile corresponding WorkloadSpread
	if oldGeneration != newGeneration {
		workloadNsn := types.NamespacedName{
			Namespace: evt.ObjectNew.GetNamespace(),
			Name:      evt.ObjectNew.GetName(),
		}
		ws, err := w.getWorkloadSpreadForWorkload(workloadNsn, gvk, workload)
		if err != nil {
			klog.Errorf("unable to get WorkloadSpread related with %s (%s/%s), err: %v",
				gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, err)
			return
		}
		if ws != nil {
			klog.V(3).Infof("%s (%s/%s) changed generation from %d to %d managed by WorkloadSpread (%s/%s)",
				gvk.Kind, workloadNsn.Namespace, workloadNsn.Name, oldGeneration, newGeneration, ws.GetNamespace(), ws.GetName())
			nsn := types.NamespacedName{Namespace: ws.GetNamespace(), Name: ws.GetName()}
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
	var workload metav1.Object
	var gvk schema.GroupVersionKind
	switch obj.(type) {
	case *appsalphav1.CloneSet:
		gvk = controllerKruiseKindCS
		workload = obj.(*appsalphav1.CloneSet)
	case *appsv1.Deployment:
		gvk = controllerKindDep
		workload = obj.(*appsv1.Deployment)
	case *appsv1.ReplicaSet:
		gvk = controllerKindRS
		workload = obj.(*appsv1.ReplicaSet)
	case *batchv1.Job:
		gvk = controllerKindJob
		workload = obj.(*batchv1.Job)
	default:
		return
	}

	workloadNsn := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	ws, err := w.getWorkloadSpreadForWorkload(workloadNsn, gvk, workload)
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

func (w *workloadEventHandler) getWorkloadSpreadForWorkload(
	workloadNamespaceName types.NamespacedName,
	gvk schema.GroupVersionKind, workload metav1.Object) (*appsalphav1.WorkloadSpread, error) {
	wsList := &appsalphav1.WorkloadSpreadList{}
	listOptions := &client.ListOptions{Namespace: workloadNamespaceName.Namespace}
	if err := w.List(context.TODO(), wsList, listOptions); err != nil {
		klog.Errorf("List WorkloadSpread failed: %s", err.Error())
		return nil, err
	}

	for _, ws := range wsList.Items {
		if ws.DeletionTimestamp != nil {
			continue
		}

		targetRef := ws.Spec.TargetReference
		if targetRef == nil {
			continue
		}

		targetGV, err := schema.ParseGroupVersion(targetRef.APIVersion)
		if err != nil {
			klog.Errorf("failed to parse targetRef's group version: %s", targetRef.APIVersion)
			continue
		}

		if targetRef.Kind == gvk.Kind && targetGV.Group == gvk.Group && targetRef.Name == workloadNamespaceName.Name {
			return &ws, nil
		}

		if workload != nil && gvk.Kind == controllerKindRS.Kind {
			ownerRef := metav1.GetControllerOf(workload)
			if ownerRef != nil {
				klog.V(3).Infof("ReplicaSet(%v): OwnerRef %v, WorkloadSpread(%s)", workloadNamespaceName, *ownerRef, ws.Name)
			} else {
				klog.V(3).Infof("ReplicaSet(%v): OwnerRef nil, WorkloadSpread(%s)", workloadNamespaceName, ws.Name)
			}
			if ownerRef != nil && ownerRef.Kind == controllerKindDep.Kind && ownerRef.Kind == targetRef.Kind &&
				ownerRef.APIVersion == targetRef.APIVersion && ownerRef.Name == targetRef.Name {
				return &ws, nil
			}
		}
	}

	return nil, nil
}
