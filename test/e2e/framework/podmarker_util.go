package framework

import (
	"context"
	"k8s.io/apimachinery/pkg/util/intstr"
	"strconv"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	markerutil "github.com/openkruise/kruise/pkg/controller/podmarker"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
)

type PodMarkerTester struct {
	c  clientset.Interface
	kc kruiseclientset.Interface
	ns string
}

func NewPodMarkerTester(c clientset.Interface, kc kruiseclientset.Interface, ns string) *PodMarkerTester {
	return &PodMarkerTester{
		c:  c,
		kc: kc,
		ns: ns,
	}
}

func (t *PodMarkerTester) DefaultEnvironment(namespace, randStr string) []*v1.Pod {
	containers := []v1.Container{
		{
			Name:    "main",
			Image:   "busybox:latest",
			Command: []string{"/bin/sh", "-c", "sleep 10000000"},
		},
	}

	return []*v1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{ // 0 ; matched for overwrite; unmatched for ignore
				Name:      "pod-0",
				Namespace: namespace,
				Labels:    map[string]string{"app": "nginx", "pod.state": "working_online", "upgradeStrategy": "rolling"},
			},
			Spec: v1.PodSpec{
				NodeSelector: map[string]string{E2eFakeKey: "true"},
				Tolerations:  []v1.Toleration{{Key: E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
				Containers:   containers,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 1 ; matched for overwrite; matched for ignore
				Name:        "pod-1",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "nginx", "pod.state": "working_online"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeSelector: map[string]string{E2eFakeKey: "true"},
				Containers:   containers,
				Tolerations:  []v1.Toleration{{Key: E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 2 ; matched for overwrite; matched for ignore;
				Name:        "pod-2",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "nginx", "pod.state": "working_online", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{markerutil.PodMarkedByPodMarkers: "marker", "markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeSelector: map[string]string{E2eFakeKey: "true"},
				Containers:   containers,
				Tolerations:  []v1.Toleration{{Key: E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 3 ; matched for overwrite; matched for ignore; should check again
				Name:        "pod-3",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "nginx"},
				Annotations: map[string]string{markerutil.PodMarkedByPodMarkers: "others"},
			},
			Spec: v1.PodSpec{
				NodeSelector: map[string]string{E2eFakeKey: "true"},
				Containers:   containers,
				Tolerations:  []v1.Toleration{{Key: E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 4 ; unmatched ; should clean
				Name:        "pod-4",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "nginx", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{markerutil.PodMarkedByPodMarkers: "marker"},
			},
			Spec: v1.PodSpec{
				Containers: containers,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 5; unmatched ; should not clean; check again
				Name:        "pod-5",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "nginx", "upgradeStrategy": "rolling"},
				Annotations: map[string]string{markerutil.PodMarkedByPodMarkers: "marker"},
			},
			Spec: v1.PodSpec{
				Containers: containers,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 6; unmatched
				Name:      "pod-6",
				Namespace: namespace,
				Labels:    map[string]string{"app": "nginx"},
			},
			Spec: v1.PodSpec{
				Containers: containers,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 7 ; unmatched; should not clean;
				Name:        "pod-7",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "nginx", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				Containers: containers,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{ // 8; unmatched; should not clean;
				Name:        "pod-8",
				Namespace:   namespace,
				Labels:      map[string]string{"app": "django", "pod.state": "working_online", "upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"markUpgradeStrategyByPodMarker": "marker"},
			},
			Spec: v1.PodSpec{
				NodeSelector: map[string]string{E2eFakeKey: "true"},
				Containers:   containers,
				Tolerations:  []v1.Toleration{{Key: E2eFakeKey, Operator: v1.TolerationOpEqual, Value: randStr, Effect: v1.TaintEffectNoSchedule}},
			},
		},
	}
}

func (t *PodMarkerTester) BasePodMarker(namespace, name string) *appsv1alpha1.PodMarker {
	replicas := intstr.FromInt(3)
	return &appsv1alpha1.PodMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1alpha1.PodMarkerSpec{
			Strategy: appsv1alpha1.PodMarkerStrategy{
				Replicas:       &replicas,
				ConflictPolicy: appsv1alpha1.PodMarkerConflictOverwrite,
			},
			MatchRequirements: appsv1alpha1.PodMarkerRequirements{
				PodSelector:  &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
				NodeSelector: &metav1.LabelSelector{MatchLabels: map[string]string{E2eFakeKey: "true"}},
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

func (t *PodMarkerTester) CreateEnvironment(pods ...*v1.Pod) (createdPods []*v1.Pod) {
	for _, p := range pods {
		pod := p.DeepCopy()
		pod, err := t.c.CoreV1().Pods(pod.Namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		createdPods = append(createdPods, pod)
		Logf("pod(%s/%s) has been created successfully", pod.Namespace, pod.Name)
	}
	return
}

func (t *PodMarkerTester) CheckDefaultEnvironment(pods []*v1.Pod) {
	scheduledToFakeNode := sets.NewInt(0, 1, 2, 3, 8)
	for _, p := range pods {
		id, err := strconv.Atoi(strings.Split(p.Name, "-")[1])
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		if scheduledToFakeNode.Has(id) {
			gomega.Eventually(func() bool {
				pod, err := t.c.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return strings.HasPrefix(pod.Spec.NodeName, fakeNodeNamePrefix)
			}, 10*time.Second, time.Second).Should(gomega.BeTrue())
		} else {
			gomega.Eventually(func() bool {
				pod, err := t.c.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return strings.HasPrefix(pod.Spec.NodeName, fakeNodeNamePrefix)
			}, 10*time.Second, time.Second).Should(gomega.BeFalse())
		}
	}
}

func (t *PodMarkerTester) GetPodMarker(namespace, name string) (*appsv1alpha1.PodMarker, error) {
	return t.kc.AppsV1alpha1().PodMarkers(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (t *PodMarkerTester) CreatePodMarker(marker *appsv1alpha1.PodMarker) *appsv1alpha1.PodMarker {
	marker, err := t.kc.AppsV1alpha1().PodMarkers(marker.Namespace).Create(context.TODO(), marker, metav1.CreateOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	t.WaitForPodMarkerCreated(marker)
	return marker
}

func (t *PodMarkerTester) UpdatePodMarker(marker *appsv1alpha1.PodMarker) *appsv1alpha1.PodMarker {
	object, err := t.kc.AppsV1alpha1().PodMarkers(marker.Namespace).Get(context.TODO(), marker.Name, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	updateError := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		object.Spec = marker.Spec
		object, err = t.kc.AppsV1alpha1().PodMarkers(marker.Namespace).Update(context.TODO(), object, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		object, _ = t.kc.AppsV1alpha1().PodMarkers(marker.Namespace).Get(context.TODO(), marker.Name, metav1.GetOptions{})
		return err
	})
	gomega.Expect(updateError).NotTo(gomega.HaveOccurred())
	return object
}

func (t *PodMarkerTester) DeletePodMarker(marker *appsv1alpha1.PodMarker) {
	err := t.kc.AppsV1alpha1().PodMarkers(marker.Namespace).Delete(context.TODO(), marker.Name, metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

func (t *PodMarkerTester) WaitForPodMarkerCreated(marker *appsv1alpha1.PodMarker) {
	pollErr := wait.PollImmediate(time.Second, time.Minute,
		func() (bool, error) {
			_, err := t.kc.AppsV1alpha1().PodMarkers(marker.Namespace).Get(context.TODO(), marker.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			return true, nil
		})
	if pollErr != nil {
		Failf("Failed waiting for PodMarker to enter running: %v", pollErr)
	}
}

func (t *PodMarkerTester) CheckMarkedPods(marker *appsv1alpha1.PodMarker, pods []*v1.Pod, indexes sets.Int) {
	for _, p := range pods {
		id, err := strconv.Atoi(strings.Split(p.Name, "-")[1])
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		if !indexes.Has(id) {
			continue
		}
		pod, err := t.c.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
		Logf("pods(%s): Labels %v, Ann: %v", pod.Name, pod.Labels, pod.Annotations)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for k, v1 := range marker.Spec.MarkItems.Labels {
			v2, ok := pod.Labels[k]
			gomega.Expect(ok).Should(gomega.BeTrue())
			gomega.Expect(v1).Should(gomega.Equal(v2))
		}
		for k, v1 := range marker.Spec.MarkItems.Annotations {
			v2, ok := pod.Annotations[k]
			gomega.Expect(ok).Should(gomega.BeTrue())
			gomega.Expect(v1).Should(gomega.Equal(v2))
		}
	}
}

func (t *PodMarkerTester) CheckCleanedPods(marker *appsv1alpha1.PodMarker, pods []*v1.Pod, indexes sets.Int) {
	for _, p := range pods {
		id, err := strconv.Atoi(strings.Split(p.Name, "-")[1])
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		if !indexes.Has(id) {
			continue
		}
		pod, err := t.c.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Name, metav1.GetOptions{})
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for k, v1 := range marker.Spec.MarkItems.Labels {
			v2, ok := pod.Labels[k]
			gomega.Expect(!ok || v1 != v2).Should(gomega.BeTrue())
		}
		for k, v1 := range marker.Spec.MarkItems.Annotations {
			v2, ok := pod.Annotations[k]
			gomega.Expect(!ok || v1 != v2).Should(gomega.BeTrue())
		}
	}
}

func (t *PodMarkerTester) CheckCornerCases(marker *appsv1alpha1.PodMarker) {
	// check pod-3
	pod, err := t.c.CoreV1().Pods(marker.Namespace).Get(context.TODO(), "pod-3", metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	markers := sets.NewString(strings.Split(pod.Annotations[markerutil.PodMarkedByPodMarkers], ",")...)
	gomega.Expect(markers.Has("others")).Should(gomega.BeTrue())

	// check pod-5
	pod, err = t.c.CoreV1().Pods(marker.Namespace).Get(context.TODO(), "pod-5", metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(pod.Labels["upgradeStrategy"]).Should(gomega.Equal("rolling"))
}
