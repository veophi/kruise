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
	rolloutDemo = &appsv1alpha1.Rollout{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appsv1alpha1.GroupVersion.String(),
			Kind:       "Rollout",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rollout",
			Namespace: "application",
		},
		Spec: appsv1alpha1.RolloutSpec{
			TargetRef: appsv1alpha1.TargetReference{
				APIVersion: apps.SchemeGroupVersion.String(),
				Kind:       "Deployment",
				Name:       "deploy",
			},
			RolloutPlan: appsv1alpha1.RolloutPlan{
				RolloutStrategy: appsv1alpha1.IncreaseFirstRolloutStrategyType,
				NumBatches:      pointer.Int32Ptr(3),
				RolloutBatches: []appsv1alpha1.RolloutBatch{
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

func TestReconcileRollout_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apps.AddToScheme(scheme)
	_ = appsv1alpha1.AddToScheme(scheme)

	rollout := rolloutDemo.DeepCopy()
	deployment := deployDemo.DeepCopy()
	replicaSet := stableReplicaSetDemo.DeepCopy()

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rollout, deployment, replicaSet).Build()
	fakeRecord := record.NewFakeRecorder(100)

	reconciler := ReconcileRollout{
		Client:   fakeClient,
		recorder: fakeRecord,
		scheme:   scheme,
	}

	_, _ = reconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{
		Name: rollout.Name, Namespace: rollout.Namespace,
	}})
}
