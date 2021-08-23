/*
Copyright 2020 The Kruise Authors.

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
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise/pkg/client/clientset/versioned"
	"github.com/openkruise/kruise/pkg/control/sidecarcontrol"
	"github.com/openkruise/kruise/pkg/util"
	"github.com/openkruise/kruise/test/e2e/framework"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/controller/history"
	utilpointer "k8s.io/utils/pointer"
)

var _ = SIGDescribe("SidecarSet", func() {
	f := framework.NewDefaultFramework("sidecarset")
	var ns string
	var c clientset.Interface
	var kc kruiseclientset.Interface
	var tester *framework.SidecarSetTester

	ginkgo.BeforeEach(func() {
		c = f.ClientSet
		kc = f.KruiseClientSet
		ns = f.Namespace.Name
		tester = framework.NewSidecarSetTester(c, kc)
	})

	framework.KruiseDescribe("SidecarSet Injecting functionality [SidecarSetInject]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all SidecarSet in cluster")
			tester.DeleteSidecarSets()
			tester.DeleteDeployments(ns)
		})

		ginkgo.It("pods don't have matched sidecarSet", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			// sidecarSet no matched pods
			sidecarSet.Spec.Selector.MatchLabels["app"] = "nomatched"
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			tester.CreateSidecarSet(sidecarSet)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// get pods
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deployment.Spec.Replicas)))
			pod := pods[0]
			gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.Containers)))
			ginkgo.By(fmt.Sprintf("test no matched sidecarSet done"))
		})

		ginkgo.It("sidecarset with volumes.downwardAPI", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			sidecarSet.Spec.Volumes = []corev1.Volume{
				{
					Name: "podinfo",
					VolumeSource: corev1.VolumeSource{
						DownwardAPI: &corev1.DownwardAPIVolumeSource{
							Items: []corev1.DownwardAPIVolumeFile{
								{
									Path: "labels",
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.labels",
									},
								},
								{
									Path: "annotations",
									FieldRef: &corev1.ObjectFieldSelector{
										FieldPath: "metadata.annotations",
									},
								},
							},
						},
					},
				},
			}
			sidecarSet.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
				{
					Name:      "podinfo",
					MountPath: "/etc/podinfo",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s with volumes.downwardAPI", sidecarSet.Name))
			tester.CreateSidecarSet(sidecarSet)
		})

		ginkgo.It("sidecarSet inject pod sidecar container", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSet.Name))
			tester.CreateSidecarSet(sidecarSet)

			// create deployment
			deployment := tester.NewBaseDeployment(ns)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deployment.Namespace, deployment.Name))
			tester.CreateDeployment(deployment)

			// get pods
			pods, err := tester.GetSelectorPods(deployment.Namespace, deployment.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			pod := pods[0]
			gomega.Expect(pod.Spec.Containers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.Containers) + len(sidecarSet.Spec.Containers)))
			gomega.Expect(pod.Spec.InitContainers).To(gomega.HaveLen(len(deployment.Spec.Template.Spec.InitContainers) + len(sidecarSet.Spec.InitContainers)))
			exceptContainers := []string{"nginx-sidecar", "main", "busybox-sidecar"}
			for i, except := range exceptContainers {
				gomega.Expect(except).To(gomega.Equal(pod.Spec.Containers[i].Name))
			}
			ginkgo.By(fmt.Sprintf("sidecarSet inject pod sidecar container done"))
		})

		ginkgo.It("sidecarSet inject pod sidecar container volumeMounts", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			// create deployment
			deployment := tester.NewBaseDeployment(ns)

			cases := []struct {
				name               string
				getDeployment      func() *apps.Deployment
				getSidecarSets     func() *appsv1alpha1.SidecarSet
				exceptVolumeMounts []string
				exceptEnvs         []string
				exceptVolumes      []string
			}{
				{
					name: "append normal volumeMounts",
					getDeployment: func() *apps.Deployment {
						deployIn := deployment.DeepCopy()
						deployIn.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:      "main-volume",
								MountPath: "/main-volume",
							},
						}
						deployIn.Spec.Template.Spec.Volumes = []corev1.Volume{
							{
								Name: "main-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return deployIn
					},
					getSidecarSets: func() *appsv1alpha1.SidecarSet {
						sidecarSetIn := sidecarSet.DeepCopy()
						sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
						sidecarSetIn.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:      "nginx-volume",
								MountPath: "/nginx-volume",
							},
						}
						sidecarSetIn.Spec.Volumes = []corev1.Volume{
							{
								Name: "nginx-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return sidecarSetIn
					},
					exceptVolumeMounts: []string{"/main-volume", "/nginx-volume"},
					exceptVolumes:      []string{"main-volume", "nginx-volume"},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)
				sidecarSetIn := cs.getSidecarSets()
				ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
				tester.CreateSidecarSet(sidecarSetIn)
				deploymentIn := cs.getDeployment()
				ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
				tester.CreateDeployment(deploymentIn)
				// get pods
				pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				// volume
				for _, volume := range cs.exceptVolumes {
					object := util.GetPodVolume(&pods[0], volume)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// volumeMounts
				sidecarContainer := &pods[0].Spec.Containers[0]
				for _, volumeMount := range cs.exceptVolumeMounts {
					object := util.GetContainerVolumeMount(sidecarContainer, volumeMount)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// envs
				for _, env := range cs.exceptEnvs {
					object := util.GetContainerEnvVar(sidecarContainer, env)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
			}
			ginkgo.By(fmt.Sprintf("sidecarSet inject pod sidecar container volumeMounts done"))
		})

		ginkgo.It("sidecarSet inject pod sidecar container volumeMounts, SubPathExpr with expanded subpath", func() {
			// create sidecarSet
			sidecarSet := tester.NewBaseSidecarSet(ns)
			// create deployment
			deployment := tester.NewBaseDeployment(ns)

			cases := []struct {
				name               string
				getDeployment      func() *apps.Deployment
				getSidecarSets     func() *appsv1alpha1.SidecarSet
				exceptVolumeMounts []string
				exceptEnvs         []string
				exceptVolumes      []string
			}{
				{
					name: "append volumeMounts SubPathExpr, volumes with expanded subpath",
					getDeployment: func() *apps.Deployment {
						deployIn := deployment.DeepCopy()
						deployIn.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:        "main-volume",
								MountPath:   "/main-volume",
								SubPathExpr: "foo/$(POD_NAME)/$(OD_NAME)/conf",
							},
						}
						deployIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
							{
								Name:  "POD_NAME",
								Value: "bar",
							},
							{
								Name:  "OD_NAME",
								Value: "od_name",
							},
						}
						deployIn.Spec.Template.Spec.Volumes = []corev1.Volume{
							{
								Name: "main-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return deployIn
					},
					getSidecarSets: func() *appsv1alpha1.SidecarSet {
						sidecarSetIn := sidecarSet.DeepCopy()
						sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
						sidecarSetIn.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
							{
								Name:      "nginx-volume",
								MountPath: "/nginx-volume",
							},
						}
						sidecarSetIn.Spec.Volumes = []corev1.Volume{
							{
								Name: "nginx-volume",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						}
						return sidecarSetIn
					},
					exceptVolumeMounts: []string{"/main-volume", "/nginx-volume"},
					exceptVolumes:      []string{"main-volume", "nginx-volume"},
					exceptEnvs:         []string{"POD_NAME", "OD_NAME"},
				},
			}

			for _, cs := range cases {
				ginkgo.By(cs.name)
				sidecarSetIn := cs.getSidecarSets()
				ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
				tester.CreateSidecarSet(sidecarSetIn)
				deploymentIn := cs.getDeployment()
				ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
				tester.CreateDeployment(deploymentIn)
				// get pods
				pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				// volume
				for _, volume := range cs.exceptVolumes {
					object := util.GetPodVolume(&pods[0], volume)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// volumeMounts
				sidecarContainer := &pods[0].Spec.Containers[0]
				for _, volumeMount := range cs.exceptVolumeMounts {
					object := util.GetContainerVolumeMount(sidecarContainer, volumeMount)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
				// envs
				for _, env := range cs.exceptEnvs {
					object := util.GetContainerEnvVar(sidecarContainer, env)
					gomega.Expect(object).ShouldNot(gomega.BeNil())
				}
			}
			ginkgo.By(fmt.Sprintf("sidecarSet inject pod sidecar container volumeMounts, SubPathExpr with expanded subpath done"))
		})

		ginkgo.It("sidecarSet inject pod sidecar container transfer Envs", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			sidecarSetIn.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "OD_NAME",
					Value: "sidecar_name",
				},
				{
					Name:  "SidecarName",
					Value: "nginx-sidecar",
				},
			}
			sidecarSetIn.Spec.Containers[0].TransferEnv = []appsv1alpha1.TransferEnvVar{
				{
					SourceContainerName: "main",
					EnvName:             "POD_NAME",
				},
				{
					SourceContainerName: "main",
					EnvName:             "OD_NAME",
				},
				{
					SourceContainerName: "main",
					EnvName:             "PROXY_IP",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "POD_NAME",
					Value: "bar",
				},
				{
					Name:  "OD_NAME",
					Value: "od_name",
				},
				{
					Name:  "PROXY_IP",
					Value: "127.0.0.1",
				},
			}
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			podIn := pods[0]
			gomega.Expect(podIn.Spec.Containers).To(gomega.HaveLen(2))
			// except envs
			exceptEnvs := map[string]string{
				"POD_NAME":    "bar",
				"OD_NAME":     "sidecar_name",
				"PROXY_IP":    "127.0.0.1",
				"SidecarName": "nginx-sidecar",
			}
			sidecarContainer := &podIn.Spec.Containers[0]
			// envs
			for key, value := range exceptEnvs {
				object := util.GetContainerEnvValue(sidecarContainer, key)
				gomega.Expect(object).To(gomega.Equal(value))
			}
			ginkgo.By(fmt.Sprintf("sidecarSet inject pod sidecar container transfer Envs done"))
		})
	})

	framework.KruiseDescribe("SidecarSet Upgrade functionality [SidecarSeUpgrade]", func() {

		ginkgo.AfterEach(func() {
			if ginkgo.CurrentGinkgoTestDescription().Failed {
				framework.DumpDebugInfo(c, ns)
			}
			framework.Logf("Deleting all SidecarSet in cluster")
			tester.DeleteSidecarSets()
			tester.DeleteDeployments(ns)
		})

		ginkgo.It("sidecarSet upgrade cold sidecar container image", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
				MaxUnavailable: &intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: 2,
				},
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = "busybox:latest"
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 60)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// get pods
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				sidecarContainer := pod.Spec.Containers[0]
				gomega.Expect(sidecarContainer.Image).To(gomega.Equal("busybox:latest"))
			}

			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container image done"))
		})

		ginkgo.It("sidecarSet upgrade cold sidecar container failed image, and only update one pod", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			sidecarSetIn, err := kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			//check pod sidecar upgrade spec annotations
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				origin := sets.String{}
				for _, sidecar := range sidecarSetIn.Spec.Containers {
					origin.Insert(sidecar.Name)
				}
				// SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
				upgradeSpec1 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashAnnotation, &pod)
				gomega.Expect(upgradeSpec1.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec1.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetRevision(sidecarSetIn)))
				target1 := sets.NewString(upgradeSpec1.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target1.List())).To(gomega.Equal(true))
				// SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
				upgradeSpec2 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashWithoutImageAnnotation, &pod)
				gomega.Expect(upgradeSpec2.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec2.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSetIn)))
				target2 := sets.NewString(upgradeSpec2.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target2.List())).To(gomega.Equal(true))
			}

			// update sidecarSet sidecar container failed image
			sidecarSetIn.Spec.Containers[0].Image = "busybox:failed"
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 60)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 0,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet sidecar container success image
			sidecarSetIn.Spec.Containers[0].Image = "busybox:latest"
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 60)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			sidecarSetIn, err = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			//check pod sidecar upgrade spec annotations
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			for _, pod := range pods {
				origin := sets.String{}
				for _, sidecar := range sidecarSetIn.Spec.Containers {
					origin.Insert(sidecar.Name)
				}
				// SidecarSetHashAnnotation = "kruise.io/sidecarset-hash"
				upgradeSpec1 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashAnnotation, &pod)
				gomega.Expect(upgradeSpec1.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec1.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetRevision(sidecarSetIn)))
				target1 := sets.NewString(upgradeSpec1.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target1.List())).To(gomega.Equal(true))
				// SidecarSetHashWithoutImageAnnotation = "kruise.io/sidecarset-hash-without-image"
				upgradeSpec2 := sidecarcontrol.GetPodSidecarSetUpgradeSpecInAnnotations(sidecarSetIn.Name, sidecarcontrol.SidecarSetHashWithoutImageAnnotation, &pod)
				gomega.Expect(upgradeSpec2.SidecarSetName).To(gomega.Equal(sidecarSetIn.Name))
				gomega.Expect(upgradeSpec2.SidecarSetHash).To(gomega.Equal(sidecarcontrol.GetSidecarSetWithoutImageRevision(sidecarSetIn)))
				target2 := sets.NewString(upgradeSpec2.SidecarList...)
				gomega.Expect(reflect.DeepEqual(origin.List(), target2.List())).To(gomega.Equal(true))
			}

			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container failed image, and only update one pod done"))
		})

		ginkgo.It("sidecarSet upgrade cold sidecar container image, and paused", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = "busybox:latest"
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 5)
			// paused
			sidecarSetIn.Spec.UpdateStrategy.Paused = true
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// paused = false, continue update pods
			sidecarSetIn.Spec.UpdateStrategy.Paused = false
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container image, and paused done"))
		})

		ginkgo.It("sidecarSet upgrade cold sidecar container image, and selector", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)
			// update pod[0] labels[canary.release] = true
			pods, err := tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			canaryPod := pods[0]
			canaryPod.Labels["canary.release"] = "true"
			tester.UpdatePod(&canaryPod)
			time.Sleep(time.Second)
			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = "busybox:latest"
			// update sidecarSet selector
			sidecarSetIn.Spec.UpdateStrategy.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"canary.release": "true",
				},
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 5)
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			time.Sleep(time.Minute)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			// check pod image
			pods, err = tester.GetSelectorPods(deploymentIn.Namespace, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(int(*deploymentIn.Spec.Replicas)))
			for _, pod := range pods {
				if _, ok := pod.Labels["canary.release"]; ok {
					sidecarContainer := pod.Spec.Containers[0]
					gomega.Expect(sidecarContainer.Image).To(gomega.Equal("busybox:latest"))
				} else {
					sidecarContainer := pod.Spec.Containers[0]
					gomega.Expect(sidecarContainer.Image).To(gomega.Equal("nginx:latest"))
				}
			}

			// update sidecarSet selector == nil, and update all pods
			sidecarSetIn.Spec.UpdateStrategy.Selector = nil
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second * 5)
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			time.Sleep(time.Minute)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container image, and selector done"))
		})

		ginkgo.It("sidecarSet upgrade cold sidecar container image, and partition", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(2)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet sidecar container
			sidecarSetIn.Spec.Containers[0].Image = "busybox:latest"
			// update sidecarSet selector
			sidecarSetIn.Spec.UpdateStrategy.Partition = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "50%",
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        2,
			}
			time.Sleep(time.Second * 10)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet partition, update all pods
			sidecarSetIn.Spec.UpdateStrategy.Partition = nil
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      2,
				UpdatedPods:      2,
				UpdatedReadyPods: 2,
				ReadyPods:        2,
			}
			time.Sleep(time.Second * 10)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container image, and partition done"))
		})

		ginkgo.It("sidecarSet upgrade cold sidecar container image, and maxUnavailable", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(4)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// update sidecarSet sidecar container
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) failed image", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = "busybox:failed"
			// update sidecarSet selector
			sidecarSetIn.Spec.UpdateStrategy.MaxUnavailable = &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "50%",
			}
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      4,
				UpdatedPods:      2,
				UpdatedReadyPods: 0,
				ReadyPods:        2,
			}
			time.Sleep(time.Second * 30)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			// update sidecarSet sidecar container
			ginkgo.By(fmt.Sprintf("update sidecarSet(%s) success image", sidecarSetIn.Name))
			sidecarSetIn.Spec.Containers[0].Image = "busybox:latest"
			tester.UpdateSidecarSet(sidecarSetIn)
			except = &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      4,
				UpdatedPods:      4,
				UpdatedReadyPods: 4,
				ReadyPods:        4,
			}
			time.Sleep(time.Second * 30)
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container image, and maxUnavailable done"))
		})

		ginkgo.It("sidecarSet update init sidecar container, and don't upgrade", func() {
			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.Spec.UpdateStrategy = appsv1alpha1.SidecarSetUpdateStrategy{
				Type: appsv1alpha1.RollingUpdateSidecarSetStrategyType,
			}
			sidecarSetIn.Spec.Containers = sidecarSetIn.Spec.Containers[:1]
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(1)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// check sidecarSet
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)
			sidecarSetIn, _ = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			hash1 := sidecarSetIn.Annotations[sidecarcontrol.SidecarSetHashAnnotation]

			// update sidecarSet sidecar container
			sidecarSetIn.Spec.InitContainers[0].Image = "busybox:failed"
			tester.UpdateSidecarSet(sidecarSetIn)
			ginkgo.By(fmt.Sprintf("update sidecarset init container image, and sidecarSet hash not changed"))
			time.Sleep(time.Second * 5)
			sidecarSetIn, _ = kc.AppsV1alpha1().SidecarSets().Get(context.TODO(), sidecarSetIn.Name, metav1.GetOptions{})
			hash2 := sidecarSetIn.Annotations[sidecarcontrol.SidecarSetHashAnnotation]
			// hash not changed
			gomega.Expect(hash1).To(gomega.Equal(hash2))
			ginkgo.By(fmt.Sprintf("sidecarSet upgrade init sidecar container, and don't upgrade done"))
		})

		ginkgo.It("sidecarSet history revision checker", func() {
			// check function
			revisionChecker := func(s *appsv1alpha1.SidecarSet, expectedCount int, expectedOrder []int64) {
				list := tester.ListControllerRevisions(s)
				// check the number of revisions
				gomega.Expect(list).To(gomega.HaveLen(expectedCount))
				for _, revision := range list {
					// check fields of revision
					mice := make(map[string]interface{})
					err := json.Unmarshal(revision.Data.Raw, &mice)
					gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
					spec := mice["spec"].(map[string]interface{})
					_, ok1 := spec["volumes"]
					_, ok2 := spec["containers"]
					_, ok3 := spec["initContainers"]
					_, ok4 := spec["imagePullSecrets"]
					gomega.Expect(ok1 && ok2 && ok3 && ok4).To(gomega.BeTrue())
				}
				if expectedOrder == nil {
					return
				}
				gomega.Expect(list).To(gomega.HaveLen(len(expectedOrder)))
				history.SortControllerRevisions(list)
				for i := range list {
					gomega.Expect(list[i].Revision).To(gomega.Equal(expectedOrder[i]))
				}
			}

			// create sidecarSet
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.SetName("e2e-test-for-history-revisions")
			sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "secret-1"}}
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)
			revisionChecker(sidecarSetIn, 1, nil)

			// update sidecarSet and stored revisions
			time.Sleep(time.Second)
			for i := 2; i <= 15; i++ {
				sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: fmt.Sprintf("secret-%d", i)}}
				tester.UpdateSidecarSet(sidecarSetIn)
				time.Sleep(time.Second)
			}
			// expected order after update
			expectedOrder := []int64{6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
			revisionChecker(sidecarSetIn, 10, expectedOrder)

			// when use older revision
			sidecarSetIn.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: fmt.Sprintf("secret-%d", 12)}}
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)
			expectedOrder = []int64{6, 7, 8, 9, 10, 11, 13, 14, 15, 16}
			revisionChecker(sidecarSetIn, 10, expectedOrder)
			ginkgo.By(fmt.Sprintf("sidecarSet upgrade cold sidecar container image, and maxUnavailable done"))
		})

		ginkgo.It("sidecarSet InjectionStrategy.Revision checker", func() {
			// create sidecarSet
			nginxName := func(tag string) string {
				return fmt.Sprintf("nginx:%s", tag)
			}
			tags := []string{
				"latest", "1.21.1", "1.21", "1.20.1", "1.20", "1.19.10",
			}
			sidecarSetIn := tester.NewBaseSidecarSet(ns)
			sidecarSetIn.SetName("e2e-test-for-injection-strategy-revision")
			sidecarSetIn.Spec.UpdateStrategy.Paused = true
			sidecarSetIn.Spec.Containers[0].Image = nginxName(tags[0])
			ginkgo.By(fmt.Sprintf("Creating SidecarSet %s", sidecarSetIn.Name))
			sidecarSetIn = tester.CreateSidecarSet(sidecarSetIn)
			// update sidecarSet and stored revisions
			time.Sleep(time.Second)
			for i := 1; i < 6; i++ {
				sidecarSetIn.Spec.Containers[0].Image = nginxName(tags[i])
				tester.UpdateSidecarSet(sidecarSetIn)
				gomega.Eventually(func() int {
					rv := tester.ListControllerRevisions(sidecarSetIn)
					return len(rv)
				}, 5*time.Second, time.Second).Should(gomega.Equal(i + 1))
			}

			// pick a history revision to inject
			pick := 3
			list := tester.ListControllerRevisions(sidecarSetIn)
			gomega.Expect(list).To(gomega.HaveLen(6))
			history.SortControllerRevisions(list)
			sidecarSetIn.Spec.InjectionStrategy.Revision = list[pick].Name
			tester.UpdateSidecarSet(sidecarSetIn)
			time.Sleep(time.Second)

			// create deployment
			deploymentIn := tester.NewBaseDeployment(ns)
			deploymentIn.Spec.Replicas = utilpointer.Int32Ptr(1)
			ginkgo.By(fmt.Sprintf("Creating Deployment(%s.%s)", deploymentIn.Namespace, deploymentIn.Name))
			tester.CreateDeployment(deploymentIn)

			// check sidecarSet revision
			pods, err := tester.GetSelectorPods(ns, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			gomega.Expect(pods[0].Spec.Containers[0].Image).To(gomega.Equal(nginxName(tags[pick])))

			// check pod sidecarSetHash
			gomega.Expect(len(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]) > 0).To(gomega.BeTrue())
			hash := make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
			err = json.Unmarshal([]byte(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]), &hash)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(hash[sidecarSetIn.Name].SidecarSetControllerRevision).To(gomega.Equal(list[pick].Name))

			// check again after sidecarSet upgrade
			sidecarSetIn.Spec.UpdateStrategy.Paused = false
			tester.UpdateSidecarSet(sidecarSetIn)
			except := &appsv1alpha1.SidecarSetStatus{
				MatchedPods:      1,
				UpdatedPods:      1,
				UpdatedReadyPods: 1,
				ReadyPods:        1,
			}
			tester.WaitForSidecarSetUpgradeComplete(sidecarSetIn, except)

			pods, err = tester.GetSelectorPods(ns, deploymentIn.Spec.Selector)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(pods).To(gomega.HaveLen(1))
			gomega.Expect(pods[0].Spec.Containers[0].Image).To(gomega.Equal(nginxName(tags[5])))
			gomega.Expect(len(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]) > 0).To(gomega.BeTrue())
			hash = make(map[string]sidecarcontrol.SidecarSetUpgradeSpec)
			err = json.Unmarshal([]byte(pods[0].Annotations[sidecarcontrol.SidecarSetHashAnnotation]), &hash)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(hash[sidecarSetIn.Name].SidecarSetControllerRevision).To(gomega.Equal(list[5].Name))
			ginkgo.By(fmt.Sprintf("sidecarSet InjectionStrategy.Revision check done"))
		})
	})
})
