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
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestPodEventHandler(t *testing.T) {
	marker1 := podMarkerDemo()
	marker2 := podMarkerDemo()
	marker1.SetName("marker-1")
	marker2.SetName("marker-2")
	marker1.Spec.MatchRequirements.PodSelector.MatchLabels = map[string]string{"app": "spring"}
	marker2.Spec.MatchRequirements.PodSelector.MatchLabels = map[string]string{"app": "mysql"}
	marker1.Spec.MatchRequirements.NodeSelector.MatchLabels = map[string]string{"arch": "x86"}
	marker2.Spec.MatchRequirements.NodeSelector.MatchLabels = map[string]string{"arch": "arm"}
	oldPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:            "pod-event",
			Namespace:       "ns-1",
			ResourceVersion: "1",
			Labels:          map[string]string{"app": "spring"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
	}
	newPod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:            "pod-event",
			Namespace:       "ns-1",
			ResourceVersion: "2",
			Labels:          map[string]string{"app": "mysql"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-2",
		},
	}
	makeEnv(marker1, marker2)
	testEnqueueRequestForPodCreate(oldPod, 1, t)

	oldPod.Labels = map[string]string{PodMarkedByPodMarkers: "marker-1"}
	newPod.Labels = map[string]string{PodMarkedByPodMarkers: "marker-2"}
	makeEnv(marker1, marker2)
	testEnqueueRequestForPodUpdate(oldPod, newPod, 2, t)

	testEnqueueRequestForPodDelete(oldPod, 1, t)
	testEnqueueRequestForPodDelete(oldPod, 1, t)
}

func testEnqueueRequestForPodCreate(pod *corev1.Pod, expectedNumber int, t *testing.T) {
	createQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	createEvt := event.CreateEvent{
		Object: pod,
	}
	testPodEventHandler.Create(createEvt, createQ)
	if createQ.Len() != expectedNumber {
		t.Errorf("unexpected create event handle queue size, expected %d actual %d", expectedNumber, createQ.Len())
	}
}

func testEnqueueRequestForPodUpdate(oldPod, newPod *corev1.Pod, expectedNumber int, t *testing.T) {
	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	updateEvt := event.UpdateEvent{
		ObjectOld: oldPod,
		ObjectNew: newPod,
	}
	testPodEventHandler.Update(updateEvt, updateQ)
	if updateQ.Len() != expectedNumber {
		t.Errorf("unexpected update event handle queue size, expected %d actual %d", expectedNumber, updateQ.Len())
	}
}

func testEnqueueRequestForPodDelete(pod *corev1.Pod, expectedNumber int, t *testing.T) {
	deleteQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	deleteEvt := event.DeleteEvent{
		Object: pod,
	}
	testPodEventHandler.Delete(deleteEvt, deleteQ)
	if deleteQ.Len() != expectedNumber {
		t.Errorf("unexpected delete event handle queue size, expected %d actual %d", expectedNumber, deleteQ.Len())
	}
}
