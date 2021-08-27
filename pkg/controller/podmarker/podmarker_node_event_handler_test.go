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

func TestNodeEventHandler(t *testing.T) {
	marker1 := podMarkerDemo()
	marker2 := podMarkerDemo()
	marker1.SetName("marker-1")
	marker2.SetName("marker-2")
	marker1.Spec.MatchRequirements.NodeSelector.MatchLabels = map[string]string{"arch": "x86"}
	marker2.Spec.MatchRequirements.NodeSelector.MatchLabels = map[string]string{"arch": "amd"}
	oldNode := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:   "node-3",
			Labels: map[string]string{"arch": "x86"}},
	}
	newNode := &corev1.Node{
		ObjectMeta: v1.ObjectMeta{
			Name:   "node-3",
			Labels: map[string]string{"arch": "amd"}},
	}
	makeEnv(marker1, marker2)
	testEnqueueRequestForNodeUpdate(oldNode, newNode, 2, t)
}

func testEnqueueRequestForNodeUpdate(oldNode, newNode *corev1.Node, expectedNumber int, t *testing.T) {
	updateQ := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	updateEvt := event.UpdateEvent{
		ObjectOld: oldNode,
		ObjectNew: newNode,
	}
	testNodeEventHandler.Update(updateEvt, updateQ)
	if updateQ.Len() != expectedNumber {
		t.Errorf("unexpected update event handle queue size, expected %d actual %d", expectedNumber, updateQ.Len())
	}
}
