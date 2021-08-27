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
	"fmt"
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	testScheme           *runtime.Scheme
	testHandler          *ReconcilePodMarker
	testPodEventHandler  *enqueueRequestForPod
	testNodeEventHandler *enqueueRequestForNode
)

func init() {
	testScheme = runtime.NewScheme()
	_ = v1.AddToScheme(testScheme)
	_ = appsv1alpha1.AddToScheme(testScheme)
}

func defaultObjects() (objects []runtime.Object) {
	pods, nodes, _ := defaultEnv()
	for _, pod := range pods {
		objects = append(objects, pod)
	}
	for _, node := range nodes {
		objects = append(objects, node)
	}
	return
}

func makeEnv(objects ...runtime.Object) {
	objects = append(objects, defaultObjects()...)
	client := fake.NewFakeClientWithScheme(testScheme, objects...)
	testHandler = &ReconcilePodMarker{
		Client: client,
	}
	testPodEventHandler = &enqueueRequestForPod{
		client: client,
	}
	testNodeEventHandler = &enqueueRequestForNode{
		client: client,
	}
}

func TestReconcile(t *testing.T) {
	marker := podMarkerDemo()
	replicas := intstr.FromInt(3)
	marker.Spec.Strategy.Replicas = &replicas
	makeEnv(marker)
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: marker.Namespace,
			Name:      marker.Name,
		},
	}
	if _, err := testHandler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("unexpected error when reconciling: %v", err)
	}

	// check status
	_ = testHandler.Client.Get(context.TODO(), types.NamespacedName{
		Namespace: marker.Namespace, Name: marker.Name}, marker)
	expectedStatus := appsv1alpha1.PodMarkerStatus{
		Desired: 3, Succeeded: 3, Failed: 0,
	}
	if !reflect.DeepEqual(marker.Status, expectedStatus) {
		t.Fatalf("expected status: %v, actual stutus: %v", expectedStatus, marker.Status)
	}

	// check the pods should be marked
	for i := 1; i <= 3; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		key := types.NamespacedName{Namespace: marker.Namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; !ok || v != "inPlace" {
			t.Fatalf("marker doesn't mark expected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; !ok || v != "marker" {
			t.Fatalf("marker doesn't mark the expected pod[%s] on annotations", podName)
		}
	}

	// check the pods should not be marked
	for i := 4; i <= 7; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		namespace := marker.Namespace
		key := types.NamespacedName{Namespace: namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; ok && v == "inPlace" {
			t.Fatalf("marker marked unexpected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; ok && v == "marker" {
			t.Fatalf("marker marked the unexpected pod[%s] on Annotations", podName)
		}
	}

	marker.Spec.Strategy.Replicas = nil
	marker.Spec.MatchRequirements.NodeSelector = nil
	if err := testHandler.Client.Update(context.TODO(), marker); err != nil {
		t.Fatalf("update podmarker err, %v", err)
	}
	if _, err := testHandler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("unexpected error when reconciling when marker.Spec.MatchRequirements.NodeSelector = nil: %v", err)
	}
	for i := 1; i <= 7; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		key := types.NamespacedName{Namespace: marker.Namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; !ok || v != "inPlace" {
			t.Fatalf("marker doesn't mark expected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; !ok || v != "marker" {
			t.Fatalf("marker doesn't mark the expected pod[%s] on annotations", podName)
		}
	}

	marker = &appsv1alpha1.PodMarker{}
	_ = testHandler.Client.Get(context.TODO(), request.NamespacedName, marker)
	marker.Spec.Strategy.Replicas = nil
	marker.Spec.MatchRequirements.PodSelector = nil
	marker.Spec.MatchRequirements.NodeSelector = podMarkerDemo().Spec.MatchRequirements.NodeSelector
	if err := testHandler.Client.Update(context.TODO(), marker); err != nil {
		t.Fatalf("update podmarker err, %v", err)
	}
	if _, err := testHandler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("unexpected error when reconciling when marker.Spec.MatchRequirements.NodeSelector = nil: %v", err)
	}
	for i := 1; i <= 4; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		key := types.NamespacedName{Namespace: marker.Namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; !ok || v != "inPlace" {
			t.Fatalf("marker doesn't mark expected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; !ok || v != "marker" {
			t.Fatalf("marker doesn't mark the expected pod[%s] on annotations", podName)
		}
	}
	// check the pods should not be marked
	for i := 5; i <= 7; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		namespace := marker.Namespace
		key := types.NamespacedName{Namespace: namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; ok && v == "inPlace" {
			t.Fatalf("marker marked unexpected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; ok && v == "marker" {
			t.Fatalf("marker marked the unexpected pod[%s] on Annotations", podName)
		}
	}
}

func TestCleanMarkBySettingReplicasAsZero(t *testing.T) {
	marker := podMarkerDemo()
	replicas := intstr.FromInt(3)
	marker.Spec.Strategy.Replicas = &replicas
	makeEnv(marker)
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: marker.Namespace,
			Name:      marker.Name,
		},
	}
	if _, err := testHandler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("unexpected error when reconciling: %v", err)
	}

	// update marker after setting replicas as zero
	marker = &appsv1alpha1.PodMarker{}
	_ = testHandler.Client.Get(context.TODO(), request.NamespacedName, marker)
	replicas = intstr.FromInt(0)
	marker.Spec.Strategy.Replicas = &replicas
	_ = testHandler.Client.Update(context.TODO(), marker)
	if _, err := testHandler.Reconcile(context.Background(), request); err != nil {
		t.Fatalf("unexpected error when reconciling: %v", err)
	}

	// check status
	marker = &appsv1alpha1.PodMarker{}
	_ = testHandler.Client.Get(context.TODO(), request.NamespacedName, marker)
	expectedStatus := appsv1alpha1.PodMarkerStatus{
		Desired: 0, Succeeded: 0, Failed: 0,
	}
	if !reflect.DeepEqual(marker.Status, expectedStatus) {
		t.Fatalf("expected status: %v, actual stutus: %v", expectedStatus, marker.Status)
	}

	// check the pods should be cleaned
	for i := 1; i <= 7; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		key := types.NamespacedName{Namespace: marker.Namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; ok && v == "inPlace" {
			t.Fatalf("marker marked unexpected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; ok && v == "marker" {
			t.Fatalf("marker marked unexpected pod[%s] on Annotations", podName)
		}
	}

	// check the pods should not be cleaned
	for i := 8; i <= 10; i++ {
		pod := &v1.Pod{}
		podName := fmt.Sprintf("pod-%d", i)
		namespace := marker.Namespace
		if i == 9 {
			namespace = "ns-2"
		}
		key := types.NamespacedName{Namespace: namespace, Name: podName}
		if err := testHandler.Client.Get(context.TODO(), key, pod); err != nil {
			t.Fatalf("unexpected error when getting pod %v", err)
		}
		if v, ok := pod.Labels["upgradeStrategy"]; !ok || v != "inPlace" {
			t.Fatalf("marker doesn't mark expected pod[%s] on Labels", podName)
		}
		if v, ok := pod.Annotations["markUpgradeStrategyByPodMarker"]; !ok || v != "marker" {
			t.Fatalf("marker doesn't mark expected pod[%s] on annotations", podName)
		}
	}
}

func TestListRelatedResourcesAndDistinguishPods(t *testing.T) {
	marker := podMarkerDemo()
	marker.Spec.Strategy.ConflictPolicy = appsv1alpha1.PodMarkerConflictIgnore
	makeEnv(marker)

	// test func ListRelatedResources
	matched, unmatched, pod2Node, err := testHandler.listRelatedResources(marker)
	if err != nil {
		t.Fatalf("unexpected error when listRelatedResources, %v", err)
	}
	if len(matched) != 4 {
		t.Fatalf("expected matched pods %d, actual %d", 4, len(matched))
	}
	if len(unmatched) != 2 {
		t.Fatalf("expected unmatched pods %d, actual %d", 2, len(unmatched))
	}

	// must ensure matched pods are sorted
	if err := sortMatchedPods(matched, pod2Node, marker.Spec.MatchPreferences); err != nil {
		t.Fatalf("unexpected error when sort matched pods, %v", err)
	}
	// test func DistinguishPods
	pm, pc := distinguishPods(matched, unmatched, marker, 999)
	if len(pm) != 2 {
		t.Fatalf("expected matched pods %d, actual %d", 2, len(pm))
	}
	if len(pc) != 2 {
		t.Fatalf("expected unmatched pods %d, actual %d", 2, len(pc))
	}
}
