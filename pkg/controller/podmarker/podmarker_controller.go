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
	"encoding/json"
	"flag"
	"reflect"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	utildiscovery "github.com/openkruise/kruise/pkg/util/discovery"
	"github.com/openkruise/kruise/pkg/util/ratelimiter"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	controllerutil "k8s.io/kubernetes/pkg/controller"
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
		Client: cli,
		scheme: mgr.GetScheme(),
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

	// Watch for changes to Node
	if err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &enqueueRequestForNode{client: mgr.GetClient()}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcilePodMarker{}

// ReconcilePodMarker reconciles a PodMarker object
type ReconcilePodMarker struct {
	client.Client
	scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.kruise.io,resources=podmarkers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.kruise.io,resources=podmarkers/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PodMarker object and makes changes based on the state read
// and what is in the PodMarker.Spec
func (r *ReconcilePodMarker) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {
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

	start := time.Now()
	klog.V(3).Infof("begin to reconcile PodMarker(%s/%s)", marker.Namespace, marker.Name)
	result, err := r.doReconcile(marker)
	klog.V(3).Infof("Reconcile PodMarker(%s/%s) done, cost time: %v", marker.Namespace, marker.Name, time.Since(start))
	return result, err
}

func (r *ReconcilePodMarker) doReconcile(podMarker *appsv1alpha1.PodMarker) (reconcile.Result, error) {
	// for the comparison before updating marker.status
	marker := podMarker.DeepCopy()

	// list related pods and nodes
	// matchedPods: the pods satisfying spec.MatchRequirements
	// unmatchedPods: the pods that do not satisfy spec.MatchRequirements, but they have been marked by this podMarker
	// podToNode: a mapping from matchedPod.Name to its' node
	matchedPods, unmatchedPods, podToNode, err := r.listRelatedResources(marker)
	if err != nil {
		klog.Errorf("Failed to list matched and unmatched pods, err: %v, name = %s/%s", err, marker.Namespace, marker.Name)
		return reconcile.Result{}, err
	}
	if matchedPods == nil && unmatchedPods == nil {
		return reconcile.Result{}, nil
	}

	// calculate the number of pods that should be marked by this PodMarker
	replicas, err := intstr.GetValueFromIntOrPercent(intstr.ValueOrDefault(marker.Spec.Strategy.Replicas, intstr.FromInt(1<<31-1)), len(matchedPods), true)
	if err != nil {
		klog.Errorf("Failed to parse .Spec.Strategy.Replicas, err: %v, name = %s/%s", err, marker.Namespace, marker.Name)
		return reconcile.Result{}, nil // no need to retry
	}

	// sort matched pods according to users' preferences
	if err := sortMatchedPods(matchedPods, podToNode, marker.Spec.MatchPreferences); err != nil {
		klog.Errorf("Failed to sort matched pods, err: %v, name = %s/%s", err, marker.Namespace, marker.Name)
		return reconcile.Result{}, nil // no need to retry
	}

	// divide related pods into podsNeedToMark and podsNeedToClean
	// this function only returns the pods that have to be marked/cleaned
	podsNeedToMark, podsNeedToClean := distinguishPods(matchedPods, unmatchedPods, marker, replicas)
	klog.V(3).Infof("replicas: %d, matchedPods(%d): %v, unmatchedPods(%d): %v, podNeedToMark(%d): %v, podNeedToClean(%d): %v",
		replicas, len(matchedPods), listPodName(matchedPods), len(unmatchedPods), listPodName(unmatchedPods),
		len(podsNeedToMark), listPodName(podsNeedToMark), len(podsNeedToClean), listPodName(podsNeedToClean))

	// mark pods
	if err = r.markPods(podsNeedToMark, marker); err != nil {
		return reconcile.Result{}, err
	}

	// clean marks for pods
	if err = r.cleanPods(podsNeedToClean, marker); err != nil {
		return reconcile.Result{}, err
	}

	// update podmarker status
	calculatePodMarkerStatus(marker)
	if !reflect.DeepEqual(marker.Status, podMarker.Status) {
		if err = r.updatePodMarkerStatus(marker); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcilePodMarker) updatePodMarkerStatus(marker *appsv1alpha1.PodMarker) error {
	newStatus := marker.Status
	by, _ := json.Marshal(&newStatus)
	klog.V(3).Infof("New status of PodMarker(%s/%s) after reconcile is %v", marker.Namespace, marker.Name, string(by))
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fetchedMarker := &appsv1alpha1.PodMarker{}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{
			Namespace: marker.Namespace, Name: marker.Name}, fetchedMarker); err != nil {
			klog.Errorf("Get error when updating PodMarker(%s/%s) status, %v", marker.Namespace, marker.Name, err)
			return err
		}
		fetchedMarker.Status = marker.Status
		return r.Client.Status().Update(context.TODO(), fetchedMarker)
	})
}

func (r *ReconcilePodMarker) listRelatedResources(marker *appsv1alpha1.PodMarker) ([]*corev1.Pod, []*corev1.Pod, map[string]*corev1.Node, error) {
	requirements := marker.Spec.MatchRequirements
	podSelector, err := metav1.LabelSelectorAsSelector(requirements.PodSelector)
	if err != nil {
		// this is an unexpected error that should be validated at webhook.
		// this error should not be returned, because it's a non transient error.
		klog.Errorf("Failed to parse requirements.PodSelector of PodMarker(%s/%s), err: %v", marker.Namespace, marker.Name, err)
		return nil, nil, nil, nil
	}
	nodeSelector, err := metav1.LabelSelectorAsSelector(requirements.NodeSelector)
	if err != nil {
		klog.Errorf("Failed to parse requirements.NodeSelector of PodMarker(%s/%s), err: %v", marker.Namespace, marker.Name, err)
		return nil, nil, nil, nil
	}

	podList := &corev1.PodList{}
	if err := r.Client.List(context.TODO(), podList, &client.ListOptions{Namespace: marker.Namespace}); err != nil {
		return nil, nil, nil, err
	}

	nodeList := &corev1.NodeList{}
	if err := r.Client.List(context.TODO(), nodeList, &client.ListOptions{LabelSelector: nodeSelector}); err != nil {
		return nil, nil, nil, err
	}

	nameToNode := make(map[string]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		nameToNode[node.Name] = node
	}

	// check whether pod matches marker.Spec.MatchRequirements
	isMatched := func(pod *corev1.Pod) bool {
		// RULE: If both of two labelSelectors are empty, nothing will be matched.
		// We make this rule is to avoid podmarker to mark all pods in cases of wrong configurations.
		// If users want to mark all pods, we provide a trick to do that, see https://openkruise.io.
		if isEmptySelector(requirements.PodSelector) && isEmptySelector(requirements.NodeSelector) {
			return false
		}
		if requirements.PodSelector != nil &&
			(podSelector.Empty() || pod.Labels == nil || !podSelector.Matches(labels.Set(pod.Labels))) {
			return false
		}
		if requirements.NodeSelector != nil &&
			(nodeSelector.Empty() || nameToNode[pod.Spec.NodeName] == nil) {
			return false
		}
		return true
	}

	matchedPods := make([]*corev1.Pod, 0)
	unmatchedPods := make([]*corev1.Pod, 0)
	podToNode := make(map[string]*corev1.Node)
	for i := range podList.Items {
		pod := &podList.Items[i]
		if !controllerutil.IsPodActive(pod) {
			klog.V(3).Infof("pod(%s/%s) is not active, then drop it", pod.Namespace, pod.Name)
			continue
		}

		if isMatched(pod) {
			matchedPods = append(matchedPods, pod)
			podToNode[pod.Name] = nameToNode[pod.Spec.NodeName]
		} else {
			// unmatchedPods only care about the related pods with this PodMarker
			usedMarkers := getMarkersWhoMarkedPod(pod)
			if usedMarkers.Has(marker.Name) {
				unmatchedPods = append(unmatchedPods, pod)
			}
		}
	}
	return matchedPods, unmatchedPods, podToNode, nil
}

func (r *ReconcilePodMarker) markPods(pods []*corev1.Pod, marker *appsv1alpha1.PodMarker) error {
	for _, p := range pods {
		pod := addMarks(p, marker)
		if err := r.Client.Update(context.TODO(), pod); err != nil {
			klog.Errorf("Failed to updates pod(%s/%s) when PodMarker(%s/%s) marking the pod, %v",
				pod.Namespace, pod.Name, marker.Namespace, marker.Name, err)
			return err
		}
		marker.Status.Succeeded++
		klog.V(3).Infof("PodMarker(%s/%s) marked pod(%s/%s) successfully", marker.Namespace, marker.Name, pod.Namespace, pod.Name)
	}
	return nil
}

func (r *ReconcilePodMarker) cleanPods(pods []*corev1.Pod, marker *appsv1alpha1.PodMarker) error {
	for _, p := range pods {
		pod := removeMarks(p, marker)
		if err := r.Client.Update(context.TODO(), pod); err != nil {
			klog.Errorf("Failed to updates pod(%s/%s) when PodMarker(%s/%s) cleaning the marks, %v",
				pod.Namespace, pod.Name, marker.Namespace, marker.Name, err)
			return err
		}
		klog.V(3).Infof("PodMarker(%s/%s) remove marks for pod(%s/%s) successfully", marker.Namespace, marker.Name, pod.Namespace, pod.Name)
	}
	return nil
}

// distinguishPods divided pods into two type: podsNeedToMark and podsNeedToClean.
// podsNeedToMark and podsNeedToClean do not contain the pods that needn't to be updated.
func distinguishPods(matchedPods, unmatchedPods []*corev1.Pod, marker *appsv1alpha1.PodMarker, desired int) (podsNeedToMark, podsNeedToClean []*corev1.Pod) {
	marker.Status.Desired = 0
	marker.Status.Succeeded = 0
	for _, pod := range matchedPods {
		usedMarkers := getMarkersWhoMarkedPod(pod)
		conflict := isConflicting(pod, &marker.Spec.MarkItems, marker.Spec.Strategy.ConflictPolicy)

		if !conflict && int(marker.Status.Desired) < desired {
			marker.Status.Desired++
			// ignore the pod that needn't to be updated.
			// in case of that the pods have been already marked by this podmarker,
			// we will not remark it again.
			if usedMarkers.Has(marker.Name) && containsEveryMarks(pod, &marker.Spec.MarkItems) {
				marker.Status.Succeeded++
				continue
			}
			podsNeedToMark = append(podsNeedToMark, pod)
		} else if usedMarkers.Has(marker.Name) && containsAnyMarks(pod, &marker.Spec.MarkItems) {
			// ignore the pod that needn't to be updated.
			// in cases of that the marks have been modified by others,
			// we WILL NOT clean these modified marks.
			podsNeedToClean = append(podsNeedToClean, pod)
		}
	}
	podsNeedToClean = append(podsNeedToClean, unmatchedPods...)
	return
}
