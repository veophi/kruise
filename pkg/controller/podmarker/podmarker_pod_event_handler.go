/*/*
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

package podmarker

import (
	"context"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	client client.Client
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addPod(q, evt)
}
func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	p.deletePod(q, evt)
}
func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt)
}

func (p *enqueueRequestForPod) addPod(q workqueue.RateLimitingInterface, evt event.CreateEvent) {
	pod, ok := evt.Object.(*v1.Pod)
	if !ok {
		return
	}
	podMarkers, err := p.getPodMarkers(pod)
	klog.V(3).Infof("Handle pod(%s/%s) create event, get matched PodMarker(%d): %v",
		pod.Namespace, pod.Name, len(podMarkers), podMarkers)
	if err != nil {
		klog.Errorf("Failed to get PodMarkers for pod(%v/%v) at create event: %v", pod.Namespace, pod.Name, err)
		return
	}
	for _, markerName := range podMarkers {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      markerName,
		}})
	}
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	oldPod, okOld := evt.ObjectOld.(*v1.Pod)
	newPod, okNew := evt.ObjectNew.(*v1.Pod)
	if !okOld || !okNew || !kubecontroller.IsPodActive(newPod) ||
		oldPod.Spec.NodeName == newPod.Spec.NodeName &&
			podutil.IsPodReady(oldPod) == podutil.IsPodReady(newPod) &&
			reflect.DeepEqual(oldPod.Labels, newPod.Labels) &&
			reflect.DeepEqual(oldPod.Annotations, newPod.Annotations) {
		return
	}

	matchedMarkers, err := p.getPodMarkers(newPod)
	if err != nil {
		klog.Errorf("Failed to get PodMarkers for pod(%v/%v) at update event: %v", newPod.Namespace, newPod.Name, err)
		return
	}
	oldMarkers := getPodAnnotationMarkers(oldPod).List()
	newMarkers := getPodAnnotationMarkers(newPod).List()
	podMarkers := sets.NewString(matchedMarkers...).
		Union(sets.NewString(oldMarkers...)).
		Union(sets.NewString(newMarkers...)).List()

	klog.V(3).Infof("Handle pod(%s/%s) update event, get matched PodMarker(%d): %v",
		newPod.Namespace, newPod.Name, len(podMarkers), podMarkers)
	for _, markerName := range podMarkers {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: newPod.Namespace,
			Name:      markerName,
		}})
	}
}

func (p *enqueueRequestForPod) deletePod(q workqueue.RateLimitingInterface, evt event.DeleteEvent) {
	pod, ok := evt.Object.(*v1.Pod)
	if !ok {
		return
	}
	podMarkers := getPodAnnotationMarkers(pod).List()
	klog.V(3).Infof("Handle pod(%s/%s) delete event, get matched PodMarker(%d): %v",
		pod.Namespace, pod.Name, len(podMarkers), podMarkers)
	for _, markerName := range podMarkers {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      markerName,
		}})
	}
}

func (p *enqueueRequestForPod) getPodMarkers(pod *v1.Pod) ([]string, error) {
	podMarkers := &appsv1alpha1.PodMarkerList{}
	if err := p.client.List(context.TODO(), podMarkers, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return nil, err
	}
	// get the corresponding node
	var node *v1.Node
	if len(pod.Spec.NodeName) != 0 {
		node = &v1.Node{}
		if err := p.client.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
			node = nil
		}
	}

	var matchedPodMarkers []string
	for _, pm := range podMarkers.Items {
		matched, err := matchesPodMarker(pod, node, &pm)
		if err != nil {
			return nil, err
		}
		if matched {
			matchedPodMarkers = append(matchedPodMarkers, pm.Name)
		}
	}
	return matchedPodMarkers, nil
}

func matchesPodMarker(pod *v1.Pod, node *v1.Node, marker *appsv1alpha1.PodMarker) (bool, error) {
	klog.V(3).Infof("pod(%s/%s) isPodActive %v, nodeName %v, matchRequirement %v",
		pod.Namespace, pod.Name, kubecontroller.IsPodActive(pod), pod.Spec.NodeName, marker.Spec.MatchRequirements)

	//if !kubecontroller.IsPodActive(pod) {
	//	return false, nil
	//}
	podSelector := marker.Spec.MatchRequirements.PodSelector
	nodeSelector := marker.Spec.MatchRequirements.NodeSelector
	if isEmptySelector(podSelector) && isEmptySelector(nodeSelector) {
		return false, nil
	}
	if podSelector != nil {
		if ok, err := objectMatchesLabelSelector(pod, podSelector); !ok || err != nil {
			return false, err
		}
	}
	if nodeSelector != nil {
		if node == nil {
			return false, nil
		}
		if ok, err := objectMatchesLabelSelector(node, nodeSelector); !ok || err != nil {
			return false, err
		}
	}
	return true, nil
}
