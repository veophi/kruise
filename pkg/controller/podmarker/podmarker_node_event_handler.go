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

package podmarker

import (
	"context"
	"reflect"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForNode{}

type enqueueRequestForNode struct {
	client client.Client
}

func (p *enqueueRequestForNode) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForNode) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForNode) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}
func (p *enqueueRequestForNode) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updateNode(q, evt)
}

func (p *enqueueRequestForNode) updateNode(q workqueue.RateLimitingInterface, evt event.UpdateEvent) {
	oldNode, okOld := evt.ObjectOld.(*v1.Node)
	newNode, okNew := evt.ObjectNew.(*v1.Node)
	if !okOld || !okNew || reflect.DeepEqual(oldNode.Labels, newNode.Labels) {
		return
	}
	p.enqueuePodMarkers(q, oldNode)
	p.enqueuePodMarkers(q, newNode)
}

func (p *enqueueRequestForNode) enqueuePodMarkers(q workqueue.RateLimitingInterface, node *v1.Node) {
	relatedMarkers, err := p.getPodMarkers(node)
	if err != nil {
		klog.Errorf("Failed to get PodMarkers for node %v/%v: %v", node.Namespace, node.Name, err)
		return
	}
	for _, marker := range relatedMarkers {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: marker.Namespace,
			Name:      marker.Name,
		}})
	}
}

func (p *enqueueRequestForNode) getPodMarkers(node *v1.Node) ([]*appsv1alpha1.PodMarker, error) {
	podMarkers := &appsv1alpha1.PodMarkerList{}
	if err := p.client.List(context.TODO(), podMarkers); err != nil {
		return nil, err
	}
	var matchedPodMarkers []*appsv1alpha1.PodMarker
	for i := range podMarkers.Items {
		pm := &podMarkers.Items[i]
		selector := pm.Spec.MatchRequirements.NodeSelector
		if isEmptySelector(selector) {
			continue
		}
		matched, err := objectMatchesLabelSelector(node, selector)
		if err != nil {
			return nil, err
		}
		if matched {
			matchedPodMarkers = append(matchedPodMarkers, pm)
		}
	}
	return matchedPodMarkers, nil
}
