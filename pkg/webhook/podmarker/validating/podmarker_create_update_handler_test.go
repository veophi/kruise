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

package validating

import (
	"context"
	"encoding/json"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	testScheme *runtime.Scheme
)

func podMarkerDemo() *appsv1alpha1.PodMarker {
	replicas := intstr.FromString("10%")
	return &appsv1alpha1.PodMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marker",
			Namespace: "default",
		},
		Spec: appsv1alpha1.PodMarkerSpec{
			Strategy: appsv1alpha1.PodMarkerStrategy{
				Replicas:       &replicas,
				ConflictPolicy: appsv1alpha1.PodMarkerConflictOverwrite,
			},
			MatchRequirements: appsv1alpha1.PodMarkerRequirements{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
				NodeSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"arch": "x86"},
				},
			},
			MatchPreferences: []appsv1alpha1.PodMarkerPreference{
				{
					PodReady: pointer.BoolPtr(true),
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"pod.phase": "running"},
					},
					NodeSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"region": "usa"},
					},
				},
			},
			MarkItems: appsv1alpha1.PodMarkerMarkItems{
				Labels:      map[string]string{"upgradeStrategy": "inPlace"},
				Annotations: map[string]string{"upgradeStrategyMarkedByPodMarker": "marker"},
			},
		},
	}
}

func init() {
	testScheme = runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(testScheme)
}

func TestValidatePodMarkerCreate(t *testing.T) {
	decoder, _ := admission.NewDecoder(testScheme)
	markerHandler := &PodMarkerCreateUpdateHandler{Decoder: decoder}

	// successful cases
	successfulPodMarker := podMarkerDemo()
	req := newAdmission(admissionv1.Create, successfulPodMarker, nil, "")
	response := markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) != 0 {
		t.Fatalf("validate failed when podMarker create, unexpected err: %v", response.Result.Message)
	}

	successfulPodMarker = podMarkerDemo()
	successfulPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "nginx"},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "upgradeStrategy",
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}
	req = newAdmission(admissionv1.Create, successfulPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) != 0 {
		t.Fatalf("validate failed when podMarker create, unexpected err: %v", response.Result.Message)
	}

	// failed cases
	failedPodMarker := podMarkerDemo()
	replicas := intstr.FromInt(-1)
	failedPodMarker.Spec.Strategy.Replicas = &replicas
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	replicas = intstr.FromString("101%")
	failedPodMarker.Spec.Strategy.Replicas = &replicas
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	replicas = intstr.FromString("!@$@#%$%")
	failedPodMarker.Spec.Strategy.Replicas = &replicas
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	failedPodMarker.Spec.Strategy.Replicas = nil
	failedPodMarker.Spec.MatchRequirements.NodeSelector = nil
	failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{}
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	failedPodMarker.Spec.Strategy.Replicas = nil
	failedPodMarker.Spec.MatchRequirements.NodeSelector = &metav1.LabelSelector{}
	failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{}
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	failedPodMarker = podMarkerDemo()
	failedPodMarker.Spec.MarkItems.Labels["app"] = "django"
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	failedPodMarker = podMarkerDemo()
	failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "nginx"},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "upgradeStrategy",
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"rolling", "inPlaceIfPossible"},
			},
		},
	}
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}

	failedPodMarker = podMarkerDemo()
	failedPodMarker.Spec.MatchRequirements.PodSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "nginx"},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "upgradeStrategy",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	}
	req = newAdmission(admissionv1.Create, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}
}

func TestValidatePodMarkerUpdate(t *testing.T) {
	// successful case
	oldPodMarker := podMarkerDemo()
	newPodMarker := podMarkerDemo()
	replicas := intstr.FromInt(7)
	newPodMarker.Spec.Strategy.Replicas = &replicas
	newPodMarker.Spec.Strategy.ConflictPolicy = appsv1alpha1.PodMarkerConflictIgnore
	decoder, _ := admission.NewDecoder(testScheme)
	markerHandler := &PodMarkerCreateUpdateHandler{Decoder: decoder}
	req := newAdmission(admissionv1.Update, newPodMarker, oldPodMarker, "")
	response := markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) != 0 {
		t.Fatalf("validate failed when podMarker update, unexpected err: %v", response.Result.Message)
	}

	// failed case
	newPodMarker.Spec.MarkItems.Annotations = nil
	req = newAdmission(admissionv1.Update, newPodMarker, oldPodMarker, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker update, expected error does not occur")
	}
}

func toRaw(marker *appsv1alpha1.PodMarker) runtime.RawExtension {
	if marker == nil {
		return runtime.RawExtension{}
	}
	by, _ := json.Marshal(marker)
	return runtime.RawExtension{Raw: by}
}

func newAdmission(op admissionv1.Operation, marker, oldMarker *appsv1alpha1.PodMarker, subResource string) admission.Request {
	object, oldObject := toRaw(marker), toRaw(oldMarker)
	return admission.Request{
		AdmissionRequest: newAdmissionRequest(op, object, oldObject, subResource),
	}
}

func newAdmissionRequest(op admissionv1.Operation, object, oldObject runtime.RawExtension, subResource string) admissionv1.AdmissionRequest {
	return admissionv1.AdmissionRequest{
		Resource:    metav1.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"},
		Operation:   op,
		Object:      object,
		OldObject:   oldObject,
		SubResource: subResource,
	}
}
