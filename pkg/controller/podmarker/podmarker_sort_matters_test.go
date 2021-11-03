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
	"fmt"
	"math/rand"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func podMarkerDemo() *appsv1alpha1.PodMarker {
	return &appsv1alpha1.PodMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marker",
			Namespace: "ns-1",
		},
		Spec: appsv1alpha1.PodMarkerSpec{
			Strategy: appsv1alpha1.PodMarkerStrategy{
				ConflictPolicy: appsv1alpha1.PodMarkerConflictOverwrite,
			},
			MatchRequirements: appsv1alpha1.PodMarkerRequirements{
				PodSelector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
				NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"arch": "x86"}},
			},
			MatchPreferences: []appsv1alpha1.PodMarkerPreference{
				{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"pod.state": "working_online"}}},
				{PodReady: pointer.BoolPtr(true)},
				{NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"arch": "x86"}}},
			},
			MarkItems: appsv1alpha1.PodMarkerMarkItems{
				Labels:      map[string]string{"upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
		},
	}
}

func defaultEnv() (pods []*v1.Pod, nodes []*v1.Node, podToNode map[string]*v1.Node) {
	pods = []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{ // matched
				Name:      "pod-1",
				Namespace: "ns-1",
				Labels:    map[string]string{"app": "nginx", "pod.state": "working_online", "upgradeStrategy": "rolling"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodRunning,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // matched
				Name:        "pod-2",
				Namespace:   "ns-1",
				Labels:      map[string]string{"app": "nginx", "pod.state": "working_online"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodRunning,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionFalse}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // matched
				Name:      "pod-3",
				Namespace: "ns-1",
				Labels: map[string]string{"app": "nginx", "pod.state": "waiting_online",
					"upgradeStrategy": "inPlace", PodMarkedByPodMarkers: "marker"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodRunning,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // matched
				Name:      "pod-4",
				Namespace: "ns-1",
				Labels:    map[string]string{"app": "nginx", PodMarkedByPodMarkers: "others"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodRunning,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionFalse}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ //unmatched
				Name:      "pod-5",
				Namespace: "ns-1",
				Labels:    map[string]string{"app": "nginx", "upgradeStrategy": "inPlace", PodMarkedByPodMarkers: "marker"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-2",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodUnknown,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionFalse}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ //unmatched
				Name:      "pod-6",
				Namespace: "ns-1",
				Labels:    map[string]string{"app": "nginx", PodMarkedByPodMarkers: "marker"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-2",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodPending,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionFalse}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ //unmatched
				Name:      "pod-7",
				Namespace: "ns-1",
				Labels:    map[string]string{"app": "nginx"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-2",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodPending,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionFalse}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ //unmatched
				Name:        "pod-8",
				Namespace:   "ns-1",
				Labels:      map[string]string{"app": "nginx", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ //unmatched
				Name:        "pod-9",
				Namespace:   "ns-2",
				Labels:      map[string]string{"app": "nginx", "pod.state": "working_online", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodRunning,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ //unmatched
				Name:        "pod-10",
				Namespace:   "ns-1",
				Labels:      map[string]string{"app": "django", "pod.state": "working_online", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeName: "node-1",
			},
			Status: v1.PodStatus{
				Phase:      v1.PodRunning,
				Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionFalse}},
			},
		},
	}

	nodes = []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-1",
				Labels: map[string]string{"arch": "x86"},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-2",
				Labels: map[string]string{"arch": "arm"},
			},
		},
	}

	podToNode = map[string]*v1.Node{
		"pod-1": nodes[0], "pod-2": nodes[0], "pod-3": nodes[0], "pod-4": nodes[0], "pod-5": nodes[1],
		"pod-6": nodes[1], "pod-7": nodes[1], "pod-8": nodes[1], "pod-9": nodes[0], "pod-10": nodes[0],
	}
	return
}

func TestSortMatchedPods(t *testing.T) {
	checker := func(sortedPods []*v1.Pod, before bool) bool {
		for i := range sortedPods {
			if i == 5 || i == 6 {
				continue
			}
			if sortedPods[i].Name != fmt.Sprintf("pod-%d", i+1) {
				return false
			}
		}
		if before {
			return sortedPods[5].Name == fmt.Sprintf("pod-%d", 6) &&
				sortedPods[6].Name == fmt.Sprintf("pod-%d", 7)
		}
		return sortedPods[5].Name == fmt.Sprintf("pod-%d", 7) &&
			sortedPods[6].Name == fmt.Sprintf("pod-%d", 6)
	}

	pods, _, podToNode := defaultEnv()
	preferences := podMarkerDemo().Spec.MatchPreferences
	for i := 0; i < 5; i++ {
		rand.Shuffle(len(pods[:8]), func(i, j int) {
			pods[i], pods[j] = pods[j], pods[i]
		})
		before := true
		for i = 0; i < 8; i++ {
			if pods[i].Name == fmt.Sprintf("pod-%d", 7) {
				before = false
				break
			} else if pods[i].Name == fmt.Sprintf("pod-%d", 6) {
				break
			}
		}
		err := sortMatchedPods(pods[:8], podToNode, preferences)
		if err != nil || !checker(pods[:8], before) {
			t.Fatalf("unexpected sequence of sorted pods, err: %v", err)
		}
	}
}
