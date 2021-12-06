package rollout

import (
	"context"
	"fmt"
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

//var (
//	scheme *runtime.Scheme
//)
//
//func init() {
//
//}

var (
	releaseDemo = &appsv1alpha1.BatchRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.GroupVersion.String(),
			Kind:       "Rollout",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rollout",
			Namespace: "application",
		},
		Spec: appsv1alpha1.BatchReleaseSpec{
			TargetRef: appsv1alpha1.TargetReference{
				APIVersion: appsv1alpha1.GroupVersion.String(),
				Kind:       "Deployment",
				Name:       "deploy",
			},
			ReleasePlan: appsv1alpha1.ReleasePlan{
				Strategy: appsv1alpha1.IncreaseFirstReleaseStrategyType,
				Batches: []appsv1alpha1.ReleaseBatch{
					{
						Replicas:       intstr.FromString("10%"),
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: int32(100)},
					},
					{
						Replicas:       intstr.FromString("50%"),
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: int32(100)},
					},
					{
						Replicas:       intstr.FromString("80%"),
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: int32(100)},
					},
				},
			},
		},
	}

	cloneDemo = &appsv1alpha1.CloneSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.SchemeGroupVersion.String(),
			Kind:       "CloneSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clone",
			Namespace: "application",
			UID:       types.UID("87076677"),
			Labels: map[string]string{
				"app": "busybox",
			},
		},
		Spec: appsv1alpha1.CloneSetSpec{
			Replicas: pointer.Int32Ptr(100),
			UpdateStrategy: appsv1alpha1.CloneSetUpdateStrategy{
				Paused:         true,
				Partition:      &intstr.IntOrString{Type: intstr.Int, IntVal: int32(1)},
				MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: int32(2)},
				MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "busybox",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: containers("latest"),
				},
			},
		},
		Status: appsv1alpha1.CloneSetStatus{
			Replicas:             100,
			ReadyReplicas:        100,
			UpdatedReplicas:      99,
			UpdatedReadyReplicas: 99,
			UpdateRevision:       "2",
			CurrentRevision:      "1",
		},
	}

	deployDemo = &apps.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apps.SchemeGroupVersion.String(),
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deploy",
			Namespace: "application",
			UID:       types.UID("87076677"),
			Labels: map[string]string{
				"app":                                "busybox",
				apps.DefaultDeploymentUniqueLabelKey: "update-pod-hash",
			},
		},
		Spec: apps.DeploymentSpec{
			Replicas: pointer.Int32Ptr(100),
			Strategy: apps.DeploymentStrategy{
				Type: apps.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &apps.RollingUpdateDeployment{
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: int32(1)},
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "busybox",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: containers("latest"),
				},
			},
		},
		Status: apps.DeploymentStatus{
			Replicas:      100,
			ReadyReplicas: 100,
		},
	}

	stableReplicaSetDemo = &apps.ReplicaSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apps.SchemeGroupVersion.String(),
			Kind:       "ReplicaSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stable-replicaset",
			Namespace: "application",
			Labels: map[string]string{
				"app":                                "busybox",
				apps.DefaultDeploymentUniqueLabelKey: "stable-pod-hash",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: deployDemo.APIVersion,
					Kind:       deployDemo.Kind,
					Name:       deployDemo.Name,
					UID:        deployDemo.UID,
					Controller: pointer.BoolPtr(true),
				},
			},
		},
		Spec: apps.ReplicaSetSpec{
			Replicas: pointer.Int32Ptr(100),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: containers("stable"),
				},
			},
		},
		Status: apps.ReplicaSetStatus{
			Replicas:      0,
			ReadyReplicas: 0,
		},
	}
)

func containers(version string) []corev1.Container {
	return []corev1.Container{
		{
			Name:  "busybox",
			Image: fmt.Sprintf("busybox:%v", version),
		},
	}
}

func TestReconcileRollout_DeploymentReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apps.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)

	release := releaseDemo.DeepCopy()
	deployment := deployDemo.DeepCopy()
	replicaSet := stableReplicaSetDemo.DeepCopy()

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(release, deployment, replicaSet).Build()
	fakeRecord := record.NewFakeRecorder(100)

	reconciler := ReconcileRollout{
		Client:   fakeClient,
		recorder: fakeRecord,
		scheme:   scheme,
	}

	_, _ = reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{
		Name: release.Name, Namespace: release.Namespace,
	}})
}

func TestReconcileRollout_CloneSetReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apps.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)

	release := releaseDemo.DeepCopy()
	release.Spec.TargetRef = appsv1alpha1.TargetReference{
		APIVersion: appsv1alpha1.SchemeGroupVersion.String(),
		Kind:       "CloneSet",
		Name:       "clone",
	}
	clone := cloneDemo.DeepCopy()

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(release, clone).Build()
	fakeRecord := record.NewFakeRecorder(100)

	reconciler := ReconcileRollout{
		Client:   fakeClient,
		recorder: fakeRecord,
		scheme:   scheme,
	}

	_, _ = reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{
		Name: release.Name, Namespace: release.Namespace,
	}})
}
