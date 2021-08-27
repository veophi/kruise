package podmarker

import (
	"sort"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	"github.com/openkruise/kruise/pkg/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
)

const (
	LimitedPreferencesNumber = 31
)

func sortMatchedPods(pods []*v1.Pod, podToNode map[string]*v1.Node, preferences []appsv1alpha1.PodMarkerPodPriority) error {
	// podPriority[pods[i]] > podPriority[pods[j]] iff pods[i] has higher priority than pods[j]
	podPriority := make(map[string]int64, len(pods))
	for i := range preferences {
		for _, pod := range pods {
			if ok, err := isPreferredPod(pod, podToNode[pod.Name], &preferences[i]); err != nil {
				return err
			} else if ok {
				// higher priority corresponds to higher bit
				podPriority[pod.Name] = podPriority[pod.Name] | (int64(1) << (LimitedPreferencesNumber-i-1))
			} else {
				podPriority[pod.Name] = 0
			}
		}
	}
	sort.Slice(pods, func(i, j int) bool {
		// If the users prefer A or B explicitly
		iPriority := podPriority[pods[i].Name]
		jPriority := podPriority[pods[j].Name]
		if iPriority > jPriority {
			return true
		}
		if iPriority < jPriority {
			return false
		}

		// If the users prefer A and B equally
		// 1.priority(Ready) > priority(NotReady)
		if podutil.IsPodReady(pods[i]) {
			return true
		}
		// 2.priority(PodRunning) > priority(PodUnknown) > priority(PodPending) > priority(Assigned) > priority(Unassigned)
		m := map[v1.PodPhase]int{"": -1, v1.PodPending: 0, v1.PodUnknown: 1, v1.PodRunning: 2}
		if m[pods[i].Status.Phase] > m[pods[j].Status.Phase] {
			return true
		} else if m[pods[i].Status.Phase] == m[pods[j].Status.Phase] {
			// check whether pods[i] is assigned
			return len(pods[i].Spec.NodeName) != 0
		} else {
			return false
		}
	})
	return nil
}

func isPreferredPod(pod *v1.Pod, node *v1.Node, preference *appsv1alpha1.PodMarkerPodPriority) (bool, error) {
	if pod == nil || (preference.PodReady == nil && preference.PodSelector == nil && preference.NodeSelector == nil) {
		return false, nil
	}
	if preference.PodReady != nil && *preference.PodReady != podutil.IsPodReady(pod) {
		return false, nil
	}
	if ok, err := objectMatchLabelSelector(pod, preference.PodSelector); !ok || err != nil {
		return false, err
	}
	if ok, err := objectMatchLabelSelector(node, preference.NodeSelector); !ok || err != nil {
		return false, err
	}
	return true, nil
}

func objectMatchLabelSelector(object metav1.Object, labelSelector *metav1.LabelSelector) (bool, error) {
	selector, err := util.GetFastLabelSelector(labelSelector)
	if err != nil {
		return false, err
	}
	if !selector.Empty() && selector.Matches(labels.Set(object.GetLabels())) {
		return true, nil
	}
	return false, nil
}

