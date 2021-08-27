package podmarker

import (
	"context"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ handler.EventHandler = &enqueueRequestForPod{}

type enqueueRequestForPod struct {
	client client.Client
}

func (p *enqueueRequestForPod) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	p.addPod(q, evt.Object.(*v1.Pod))
}

func (p *enqueueRequestForPod) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	if pod, ok := evt.Object.(*v1.Pod); ok {
		p.deletePod(q, pod)
	}
}

func (p *enqueueRequestForPod) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (p *enqueueRequestForPod) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	p.updatePod(q, evt.ObjectOld.(*v1.Pod), evt.ObjectNew.(*v1.Pod))
}

func (p *enqueueRequestForPod) addPod(q workqueue.RateLimitingInterface, pod *v1.Pod) {
	if pod.DeletionTimestamp != nil {
		p.deletePod(q, pod)
		return
	}

	podMarkers, err := p.getPodMarkers(pod, true)
	if err != nil {
		klog.Errorf("Failed to get PodMarkers for pod %v/%v: %v", pod.Namespace, pod.Name, err)
		return
	}

	for _, markerName := range podMarkers {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      markerName,
		}})
	}
}

func (p *enqueueRequestForPod) updatePod(q workqueue.RateLimitingInterface, oldPod, newPod *v1.Pod) {
	if oldPod.ResourceVersion == newPod.ResourceVersion {
		return
	}

	matchedMarkers, err := p.getPodMarkers(newPod, true)
	if err != nil {
		klog.Errorf("Failed to get PodMarkers for pod %v/%v: %v", newPod.Namespace, newPod.Name, err)
		return
	}

	oldOwnedMarkers, _ := p.getPodMarkers(oldPod, false)
	newOwnedMarkers, _ := p.getPodMarkers(newPod, false)
	podMarkers := sets.NewString(matchedMarkers...).
		Union(sets.NewString(newOwnedMarkers...)).
		Union(sets.NewString(oldOwnedMarkers...))

	for _, markerName := range podMarkers.List() {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: newPod.Namespace,
			Name:      markerName,
		}})
	}
}

func (p *enqueueRequestForPod) deletePod(q workqueue.RateLimitingInterface, pod *v1.Pod) {
	ownedMarkers, _ := p.getPodMarkers(pod, false)
	for _, markerName := range ownedMarkers {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      markerName,
		}})
	}
}

func (p *enqueueRequestForPod) getPodMarkers(pod *v1.Pod, recalculate bool) ([]string, error) {
	if !recalculate {
		if pod.Annotations != nil {
			return strings.Split(pod.Annotations[PodMarkedByPodMarkers], ","), nil
		} else {
			return nil, nil
		}
	}
	podMarkers := &appsv1alpha1.PodMarkerList{}
	if err := p.client.List(context.TODO(), podMarkers, &client.ListOptions{Namespace: pod.Namespace}); err != nil {
		return nil, err
	}
	var node *v1.Node = nil
	if len(pod.Spec.NodeName) != 0 {
		node = &v1.Node{}
		if err := p.client.Get(context.TODO(), types.NamespacedName{
			Namespace: pod.Namespace, Name: pod.Spec.NodeName}, node); err != nil {
			return nil, err
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
	if !kubecontroller.IsPodActive(pod) {
		return false, nil
	}
	if ok, err := objectMatchLabelSelector(pod, &marker.Spec.MatchRequirements.PodSelector); !ok || err != nil {
		return false, err
	}
	if node == nil {
		return true, nil
	}
	if ok, err := objectMatchLabelSelector(node, &marker.Spec.MatchRequirements.NodeSelector); !ok || err != nil {
		return false, err
	}
	return true, nil
}