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

package apps

import (
	"context"
	markerutil "github.com/openkruise/kruise/pkg/controller/podmarker"
	v1 "k8s.io/api/core/v1"
	"strings"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = SIGDescribe("PodMarker", func() {
	f := framework.NewDefaultFramework("podmarkers")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.PodMarkerTester
	var nodeTester *framework.NodeTester
	var randStr string

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewPodMarkerTester(c, kc, ns)
		nodeTester = framework.NewNodeTester(c)
		randStr = rand.String(10)
	})

	ginkgo.AfterEach(func() {
		err := nodeTester.DeleteFakeNode(randStr)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	framework.KruiseDescribe("PodMarker", func() {
		ginkgo.It("PodMarker life cycle checker", func() {
			ginkgo.By("Create Fake Node " + randStr)
			node, err := nodeTester.CreateFakeNode(randStr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			marker := tester.BasePodMarker(ns, "marker")
			pods := tester.DefaultEnvironment(ns, randStr)
			_ = tester.CreateEnvironment(pods...)
			tester.CheckDefaultEnvironment(pods)

			// # case 1:  check after PodMarker create
			ginkgo.By("check after PodMarker create...")
			tester.CreatePodMarker(marker)
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(3)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(0, 1, 2, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(3, 4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 2: check after Marked Pod Deletion
			ginkgo.By("check after Marked Pod Deletion...")
			err = f.PodClient().Delete(context.TODO(), pods[0].Name, metav1.DeleteOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() bool {
				pod, err := f.PodClient().Get(context.TODO(), pods[3].Name, metav1.GetOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				markerNames := pod.Annotations[markerutil.PodMarkedByPodMarkers]
				return sets.NewString(strings.Split(markerNames, ",")...).Has(marker.Name)
			}, 50*time.Second, time.Second).Should(gomega.BeTrue())
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(3)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(1, 2, 3, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 3: check after setting replicas as zero
			ginkgo.By("check after setting replicas as zero...")
			replicas := intstr.FromInt(0)
			marker.Spec.Strategy.Replicas = &replicas
			tester.UpdatePodMarker(marker)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(0)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(1, 2, 3, 4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 4: check after setting replicas as nil
			ginkgo.By("check after setting replicas as nil...")
			marker.Spec.Strategy.Replicas = nil
			tester.UpdatePodMarker(marker)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(3)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(1, 2, 3, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 5: check after matched pods create
			ginkgo.By("check after matched pods create...")
			_, err = f.PodClient().Create(context.TODO(), pods[0], metav1.CreateOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int {
				p, err := f.PodClient().Get(context.TODO(), pods[0].Name, metav1.GetOptions{})
				if err != nil {
					return 0
				}
				return len(p.Spec.NodeName)
			}, 20*time.Second, time.Second).ShouldNot(gomega.Equal(0))
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(4)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(0, 1, 2, 3, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 6: check after updating node labels
			ginkgo.By("check after updating node labels...")
			updatedNode := node.DeepCopy()
			updatedNode.Labels[framework.E2eFakeKey] = "false"
			_, err = nodeTester.UpdateNodeLabels(updatedNode)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(0)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(1, 2, 3, 4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 7: check after updating node labels back
			ginkgo.By("check after updating node labels back...")
			_, err = nodeTester.UpdateNodeLabels(node)
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(4)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(0, 1, 2, 3, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(4, 5, 6))
			tester.CheckCornerCases(marker)

			// # case 8: check after setting ConflictPolicy as "Ignore"
			ginkgo.By("check after setting ConflictPolicy as \"Ignore\"...")
			marker.Spec.Strategy.ConflictPolicy = appsv1alpha1.PodMarkerConflictIgnore
			tester.UpdatePodMarker(marker)
			f.PodClient().Update(pods[0].Name, func(pod *v1.Pod) {
				pod.Labels = pods[0].Labels
				pod.Annotations = pods[0].Annotations
			})
			f.PodClient().Update(pods[3].Name, func(pod *v1.Pod) {
				pod.Labels = pods[0].Labels
				pod.Annotations = pods[0].Annotations
			})
			gomega.Eventually(func() int32 {
				marker, err = tester.GetPodMarker(marker.Namespace, marker.Name)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				return marker.Status.Succeeded
			}, 20*time.Second, time.Second).Should(gomega.Equal(int32(2)))
			gomega.Expect(marker.Status.Desired).Should(gomega.Equal(marker.Status.Succeeded))
			tester.CheckMarkedPods(marker, pods, sets.NewInt(1, 2, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(0, 3, 4, 5, 6))

			// # case 9: check after podMarker delete
			ginkgo.By("check after podMarker delete...")
			tester.DeletePodMarker(marker)
			tester.CheckMarkedPods(marker, pods, sets.NewInt(1, 2, 7, 8))
			tester.CheckCleanedPods(marker, pods, sets.NewInt(0, 3, 4, 5, 6))
			ginkgo.By("Done!")
		})
	})
})
