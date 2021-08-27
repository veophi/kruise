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
	"flag"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"reflect"
	"strings"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func init() {
	flag.IntVar(&concurrentReconciles, "podmarker-workers", concurrentReconciles, "Max concurrent workers for PodMarker controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("PodMarker")
)

const (
	PodMarkedByPodMarkers string = "kruise.io/marked-by-podmarker"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new PodMarker Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	cli := util.NewClientFromManager(mgr, "podmarker-controller")
	return &ReconcilePodMarker{
		Client:    cli,
		scheme:    mgr.GetScheme(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("podmarker-controller", mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter: ratelimiter.DefaultControllerRateLimiter()})
	if err != nil {
		return err
	}

	// Watch for changes to PodMarker
	err = c.Watch(&source.Kind{Type: &appsv1alpha1.PodMarker{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldMarker := e.ObjectOld.(*appsv1alpha1.PodMarker)
			newMarker := e.ObjectNew.(*appsv1alpha1.PodMarker)
			if !reflect.DeepEqual(oldMarker.Spec, newMarker.Spec) {
				klog.V(3).Infof("Observed updated Spec for PodMarker: %s/%s", newMarker.Namespace, newMarker.Name)
				return true
			}
			return false
		},
	})
	if err != nil {
		return err
	}

	// Watch for changes to Pod
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &enqueueRequestForPod{client: mgr.GetClient()}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePodMarker{}

// ReconcilePodMarker reconciles a PodMarker object
type ReconcilePodMarker struct {
	client.Client
	scheme             *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=podmarkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=podmarkers/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PodMarker object and makes changes based on the state read
// and what is in the PodMarker.Spec
func (r *ReconcilePodMarker) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the PodMarker instance
	marker := &appsv1alpha1.PodMarker{}
	err := r.Get(context.TODO(), request.NamespacedName, marker)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	klog.V(3).Infof("begin to process podmarker %v for reconcile", marker.Name)
	return r.doReconcile(marker)
}

func (r *ReconcilePodMarker) doReconcile(marker *appsv1alpha1.PodMarker) (reconcile.Result, error) {
	// 1.list related pods and their nodes
	// matchedPods: the pods satisfying spec.MatchRequirements
	// unmatchedPods: the pods that do not satisfy spec.MatchRequirements, but they have been marked by this podMarker
	// podToNode: the corresponding nodes of matchedPods
	matchedPods, unmatchedPods, podToNode, err := r.listRelatedResources(marker)
	if err != nil {
		return reconcile.Result{}, err
	}
	total := len(matchedPods)

	// 2.calculate the number of pods should be marked
	targets, err :=  intstr.GetValueFromIntOrPercent( intstr.ValueOrDefault(marker.Spec.Strategy.Replicas, intstr.FromInt(1<<31-1)) , total, true)
	if err != nil {
		klog.Errorf("Failed to parse .Spec.Strategy.Replicas, err %v", err)
		return reconcile.Result{}, nil // no need to retry
	}

	// 3.sort matched pods according to user's preferences
	if err := sortMatchedPods(matchedPods, podToNode, marker.Spec.MatchPreferences); err != nil {
		return reconcile.Result{}, nil // no need to retry
	}

	// 4.divide related pods into podsNeedToMark and podsNeedToClean
	podsNeedToMark, podsNeedToClean := distinguishPods(matchedPods, unmatchedPods, marker, targets)
	oldStatus := marker.Status.DeepCopy() // for the comparison when update status

	// 5.mark pods
	if err = r.markPods(podsNeedToMark, marker); err != nil {
		klog.Errorf("Failed to mark pods for %s/%s, err: %v", marker.Namespace, marker.Name, err)
	}

	// 6.here only clean the pods that have been marked by this podmarker
	if err = r.cleanPods(podsNeedToClean, marker); err != nil {
		klog.Errorf("Failed to clean pods for %s/%s, err: %v", marker.Namespace, marker.Name, err)
	}

	// 7. update podmarker status
	calculatePodMarkerStatus(marker, targets)
	if !reflect.DeepEqual(marker.Status, oldStatus) {
		if err = r.updatePodMarkerStatus(marker); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, err
}

func (r *ReconcilePodMarker) updatePodMarkerStatus(marker *appsv1alpha1.PodMarker) error {
	newStatus := marker.Status.DeepCopy()
	newMarker := &appsv1alpha1.PodMarker{}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: marker.Namespace, Name: marker.Name}, newMarker); err != nil {
			return err
		}
		newMarker.Status = *newStatus
		return r.Client.Status().Update(context.TODO(), newMarker)
	})
}

func calculatePodMarkerStatus(marker *appsv1alpha1.PodMarker, desired int) {
	marker.Status.Desired = int32(desired)
	marker.Status.Failed = int32(desired) - marker.Status.Failed
}

func (r *ReconcilePodMarker) listRelatedResources(marker *appsv1alpha1.PodMarker)([]*corev1.Pod, []*corev1.Pod, map[string]*corev1.Node, error) {
	requirements := marker.Spec.MatchRequirements
	podSelector, err := util.GetFastLabelSelector(&requirements.NodeSelector)
	if err != nil {
		return nil, nil, nil, err
	}
	nodeSelector, err := util.GetFastLabelSelector(&requirements.NodeSelector)
	if err != nil {
		return nil, nil, nil, err
	}

	podList := &corev1.PodList{}
	if err := r.Client.List(context.TODO(), podList, &client.ListOptions{Namespace: marker.Namespace}); err != nil {
		return nil, nil, nil, err
	}

	nodeList := &corev1.NodeList{}
	if err := r.Client.List(context.TODO(), nodeList, &client.ListOptions{Namespace: marker.Namespace, LabelSelector: nodeSelector}); err != nil {
		return nil, nil, nil, err
	}

	nameToNode := make(map[string]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		nameToNode[node.Name] = node
	}

	requirementForNode := len(requirements.NodeSelector.MatchLabels) + len(requirements.NodeSelector.MatchExpressions)
	matchesPod := func(pod *corev1.Pod) bool {
		if !podSelector.Empty() && podSelector.Matches(labels.Set(pod.GetLabels())) {
			return false
		}
		if requirementForNode == 0 {
			return true
		}
		return nameToNode[pod.Spec.NodeName] != nil
	}

	matchedPods := make([]*corev1.Pod, 0)
	unmatchedPods := make([]*corev1.Pod, 0)
	podToNode := make(map[string]*corev1.Node)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if matchesPod(pod) {
			matchedPods = append(matchedPods, pod)
			podToNode[pod.Name] = nameToNode[pod.Spec.NodeName]
		} else if pod.Annotations != nil {
			usedMarkers := sets.NewString(strings.Split(pod.Annotations[PodMarkedByPodMarkers], ",")...)
			if usedMarkers.Has(marker.Name) {
				unmatchedPods = append(unmatchedPods, pod)
			}
		}
	}
	return matchedPods, unmatchedPods, podToNode, nil
}

func (r *ReconcilePodMarker) markPods(pods []*corev1.Pod, marker *appsv1alpha1.PodMarker) error {
	if len(pods) == 0 {
		return nil
	}
	markItems := marker.Spec.MarkItems
	for _, p := range pods {
		pod := p.DeepCopy()
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}

		// check whether pod has been marked by this PodMarker
		usedMarkers := sets.NewString(strings.Split(pod.Annotations[PodMarkedByPodMarkers], ",")...)
		if usedMarkers.Has(marker.Name) {
			continue
		}

		for k, v := range markItems.Labels {
			pod.Labels[k] = v
		}
		for k, v := range markItems.Annotations {
			pod.Annotations[k] = v
		}
		if err := r.Client.Update(context.TODO(), pod); err != nil {
			return err
		}
		marker.Status.Succeeded++
	}
	return nil
}

func (r *ReconcilePodMarker) cleanPods(pods []*corev1.Pod, marker *appsv1alpha1.PodMarker) error {
	if len(pods) == 0 {
		return nil
	}
	markItems := marker.Spec.MarkItems
	for _, p := range pods {
		pod := p.DeepCopy()
		for k := range markItems.Labels {
			delete(pod.Labels, k)
		}
		for k := range markItems.Annotations {
			delete(pod.Annotations, k)
		}
		if err := r.Client.Update(context.TODO(), pod); err != nil {
			return err
		}
		marker.Status.Succeeded--
	}
	return nil
}

func distinguishPods(matchedPods, unmatchedPods []*corev1.Pod, marker *appsv1alpha1.PodMarker, targets int) (podsNeedToMark, podsNeedToClean []*corev1.Pod){
	checkConflict := func(pod *corev1.Pod, markItems *appsv1alpha1.PodMarkerMarkItems, policy appsv1alpha1.PodMarkerConflictPolicyType) bool {
		if policy == appsv1alpha1.PodMarkerConflictOverwrite {
			return false
		}
		conflict := false
		for k, v := range markItems.Labels {
			if pod.Labels != nil && pod.Labels[k] != v {
				conflict = true
				break
			}
		}
		for k, v := range markItems.Annotations {
			if pod.Annotations != nil && pod.Annotations[k] != v {
				conflict = true
				break
			}
		}
		return conflict
	}

	selected := 0
	for _, pod := range matchedPods {
		conflict := checkConflict(pod, &marker.Spec.MarkItems, marker.Spec.Strategy.ConflictPolicy)
		usedMarkers := sets.NewString(strings.Split(pod.Annotations[PodMarkedByPodMarkers], ",")...)
		if pod.Annotations == nil {
			pod.Annotations = make(map[string]string)
		}
		if !conflict && selected < targets {
			selected++
			podsNeedToMark = append(podsNeedToMark, pod)
			usedMarkers.Insert(marker.Name)
			pod.Annotations[PodMarkedByPodMarkers] = strings.Join(usedMarkers.List(), ",")
		} else if selected >= targets && usedMarkers.Has(marker.Name) {
			podsNeedToClean = append(podsNeedToClean, pod)
			usedMarkers.Delete(marker.Name)
			pod.Annotations[PodMarkedByPodMarkers] = strings.Join(usedMarkers.List(), ",")
		}
	}
	podsNeedToClean = append(podsNeedToClean, unmatchedPods...)
	return
}