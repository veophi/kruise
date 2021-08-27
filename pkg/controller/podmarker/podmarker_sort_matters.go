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
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	markerutil "github.com/openkruise/kruise/pkg/webhook/podmarker/validating"

	v1 "k8s.io/api/core/v1"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	kubecontroller "k8s.io/kubernetes/pkg/controller"
)

func sortMatchedPods(pods []*v1.Pod, podToNode map[string]*v1.Node, preferences []appsv1alpha1.PodMarkerPreference) error {
	// podPriority[pods[i]] > podPriority[pods[j]] iff pods[i] has higher priority than pods[j]
	podPriority := make(map[string]int64, len(pods))
	for i := range preferences {
		for _, pod := range pods {
			if ok, err := isPreferredPod(pod, podToNode[pod.Name], &preferences[i]); err != nil {
				return err
			} else if ok {
				// higher the bit is, higher the priority is
				podPriority[pod.Name] |= int64(1) << (markerutil.LimitedPreferencesNumber - i - 1)
			}
		}
	}

	sort.SliceStable(pods, func(i, j int) bool {
		// If users prefer I or J explicitly
		iPriority := podPriority[pods[i].Name]
		jPriority := podPriority[pods[j].Name]
		if iPriority > jPriority {
			return true
		}
		if iPriority < jPriority {
			return false
		}

		// If users prefer I and J equally
		// Sort Pods with default sequence, priority:
		//	- Unassigned < assigned
		//	- PodPending < PodUnknown < PodRunning
		//	- Not ready < ready
		//	- Been ready for empty time < less time < more time
		//	- Pods with containers with higher restart counts < lower restart counts
		//	- Empty creation time pods < newer pods < older pods
		return kubecontroller.ActivePods(pods).Less(j, i)
	})
	return nil
}

func isPreferredPod(pod *v1.Pod, node *v1.Node, preference *appsv1alpha1.PodMarkerPreference) (bool, error) {
	if pod == nil || preference == nil {
		return false, fmt.Errorf("found an unexpected empty pointer when calculating preferred pods")
	}
	// check podReady
	if preference.PodReady != nil && *preference.PodReady != podutil.IsPodReady(pod) {
		return false, nil
	}
	// check podSelector
	if preference.PodSelector != nil {
		if ok, err := objectMatchesLabelSelector(pod, preference.PodSelector); !ok || err != nil {
			return false, err
		}
	}
	// check nodeSelector
	if preference.NodeSelector != nil {
		if node == nil {
			return false, nil
		}
		if ok, err := objectMatchesLabelSelector(node, preference.NodeSelector); !ok || err != nil {
			return false, err
		}
	}
	return true, nil
}
