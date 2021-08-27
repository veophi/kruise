package podmarker

import (
	"fmt"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
)

// object must be not nil
func objectMatchesLabelSelector(object metav1.Object, labelSelector *metav1.LabelSelector) (bool, error) {
	if object.GetLabels() == nil {
		return false, nil
	}
	selector, err := util.GetFastLabelSelector(labelSelector)
	if err != nil {
		return false, err
	}
	if !selector.Empty() && selector.Matches(labels.Set(object.GetLabels())) {
		return true, nil
	}
	return false, nil
}

func containsAnyMarks(pod *corev1.Pod, marks *appsv1alpha1.PodMarkerMarkItems) bool {
	if pod.Labels != nil {
		for k, v1 := range marks.Labels {
			if v2, ok := pod.Labels[k]; ok && v1 == v2 {
				return true
			}
		}
	}
	if pod.Annotations != nil {
		for k, v1 := range marks.Annotations {
			if v2, ok := pod.Annotations[k]; ok && v1 == v2 {
				return true
			}
		}
	}
	return false
}

func containsEveryMarks(pod *corev1.Pod, marks *appsv1alpha1.PodMarkerMarkItems) bool {
	if (pod.Labels == nil && marks.Labels != nil) || (pod.Annotations == nil && marks.Annotations != nil) {
		return false
	}
	for k, v1 := range marks.Labels {
		if v2, ok := pod.Labels[k]; !ok || v1 != v2 {
			return false
		}
	}
	for k, v1 := range marks.Annotations {
		if v2, ok := pod.Annotations[k]; !ok || v1 != v2 {
			return false
		}
	}
	return true
}

func hasConflict(pod *corev1.Pod, markItems *appsv1alpha1.PodMarkerMarkItems, policy appsv1alpha1.PodMarkerConflictPolicyType) bool {
	if policy == appsv1alpha1.PodMarkerConflictOverwrite {
		return false
	}
	if pod.Labels != nil {
		for k, v1 := range markItems.Labels {
			if v2, ok := pod.Labels[k]; ok && v1 != v2 {
				return true
			}
		}
	}
	if pod.Annotations != nil {
		for k, v1 := range markItems.Annotations {
			if v2, ok := pod.Annotations[k]; ok && v1 != v2 {
				return true
			}
		}
	}
	return false
}

func getMarkPatch(pod *corev1.Pod, marker *appsv1alpha1.PodMarker) []byte {
	var ls, as []string
	for k, v := range marker.Spec.MarkItems.Labels {
		ls = append(ls, fmt.Sprintf(`"%s":"%s"`, k, v))
	}
	for k, v := range marker.Spec.MarkItems.Annotations {
		as = append(as, fmt.Sprintf(`"%s":"%s"`, k, v))
	}

	usedMarkers := getPodAnnotationMarkers(pod)
	usedMarkers.Insert(marker.Name)
	markers := strings.Join(usedMarkers.List(), ",")
	as = append(as, fmt.Sprintf(`"%s":"%s"`, PodMarkedByPodMarkers, markers))

	return []byte(fmt.Sprintf(`{"metadata":{"labels":{%s},"annotations":{%s}}}`,
		strings.Join(ls, ","), strings.Join(as, ",")))
}

func getCleanPatch(pod *corev1.Pod, marker *appsv1alpha1.PodMarker) []byte {
	var ls, as []string
	pl := pod.GetLabels()
	pa := pod.GetAnnotations()
	if pl != nil {
		for k, v1 := range marker.Spec.MarkItems.Labels {
			if v2, ok := pl[k]; ok && v1 == v2 {
				ls = append(ls, fmt.Sprintf(`"%s":null`, k))
			}
		}
	}
	if pa != nil {
		for k, v1 := range marker.Spec.MarkItems.Annotations {
			if v2, ok := pa[k]; ok && v1 == v2 {
				as = append(as, fmt.Sprintf(`"%s":null`, k))
			}
		}

		usedMarkers := getPodAnnotationMarkers(pod)
		usedMarkers.Delete(marker.Name)
		if usedMarkers.Len() > 0 {
			markers := strings.Join(usedMarkers.List(), ",")
			as = append(as, fmt.Sprintf(`"%s":"%s"`, PodMarkedByPodMarkers, markers))
		} else {
			as = append(as, fmt.Sprintf(`"%s":null`, PodMarkedByPodMarkers))
		}
	}
	return []byte(fmt.Sprintf(`{"metadata":{"labels":{%s},"annotations":{%s}}}`,
		strings.Join(ls, ","), strings.Join(as, ",")))
}

func calculatePodMarkerStatus(marker *appsv1alpha1.PodMarker) {
	marker.Status.ObservedGeneration = marker.Generation
	marker.Status.Failed = marker.Status.Desired - marker.Status.Succeeded
}

func isEmptySelector(selector *metav1.LabelSelector) bool {
	if selector == nil {
		return true
	}
	return len(selector.MatchLabels)+len(selector.MatchExpressions) == 0
}

func getPodAnnotationMarkers(pod *corev1.Pod) sets.String {
	if pod.Annotations == nil || len(pod.Annotations[PodMarkedByPodMarkers]) == 0 {
		return sets.NewString()
	}
	return sets.NewString(strings.Split(pod.Annotations[PodMarkedByPodMarkers], ",")...)
}

func listPodName(pods []*corev1.Pod) (names []string) {
	for _, p := range pods {
		names = append(names, p.Name)
	}
	return
}
