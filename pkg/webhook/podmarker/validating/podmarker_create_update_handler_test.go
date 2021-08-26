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

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	scheme    *runtime.Scheme
	podMarker = appsv1alpha1.PodMarker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "marker",
			Namespace: "default",
		},
		Spec: appsv1alpha1.PodMarkerSpec{
			Strategy: appsv1alpha1.PodMarkerStrategy{
				ConflictPolicy: appsv1alpha1.PodMarkerConflictOverwrite,
			},
			MatchRequirements: appsv1alpha1.PodMarkerRequirements{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "nginx"},
				},
				NodeSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"arch": "x86"},
				},
			},
			MatchPreferences: []appsv1alpha1.PodMarkerPodPriority{
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
)

func init() {
	scheme = runtime.NewScheme()
	_ = appsv1alpha1.AddToScheme(scheme)
	replicas := intstr.FromString("10%")
	podMarker.Spec.Strategy.Replicas = &replicas
}

func TestValidatePodMarkerCreate(t *testing.T) {
	decoder, _ := admission.NewDecoder(scheme)
	markerHandler := &PodMarkerCreateUpdateHandler{Decoder: decoder}
	req := newAdmission(admissionv1beta1.Create, &podMarker, nil, "")

	// successful case
	response := markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) != 0 {
		t.Fatalf("validate failed when podMarker create, unexpected err: %v", response.Result.Message)
	}

	// failed case
	failedPodMarker := podMarker.DeepCopy()
	preferences := failedPodMarker.Spec.MatchPreferences
	preferences = append(preferences, appsv1alpha1.PodMarkerPodPriority{PodReady: pointer.BoolPtr(true)})
	preferences = append(preferences, appsv1alpha1.PodMarkerPodPriority{PodReady: pointer.BoolPtr(false)})
	failedPodMarker.Spec.MatchPreferences = preferences
	req = newAdmission(admissionv1beta1.Update, failedPodMarker, nil, "")
	response = markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) == 0 {
		t.Fatalf("validate failed when podMarker create, expected error does not occur")
	}
}

func TestValidatePodMarkerUpdate(t *testing.T) {
	// successful case
	oldPodMarker := &podMarker
	newPodMarker := podMarker.DeepCopy()
	replicas := intstr.FromInt(7)
	newPodMarker.Spec.Strategy.Replicas = &replicas
	newPodMarker.Spec.Strategy.ConflictPolicy = appsv1alpha1.PodMarkerConflictIgnore
	decoder, _ := admission.NewDecoder(scheme)
	markerHandler := &PodMarkerCreateUpdateHandler{Decoder: decoder}
	req := newAdmission(admissionv1beta1.Update, newPodMarker, oldPodMarker, "")
	response := markerHandler.Handle(context.TODO(), req)
	if len(response.Result.Message) != 0 {
		t.Fatalf("validate failed when podMarker update, unexpected err: %v", response.Result.Message)
	}

	// failed case
	newPodMarker.Spec.MarkItems.Annotations = nil
	req = newAdmission(admissionv1beta1.Update, newPodMarker, oldPodMarker, "")
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

func newAdmission(op admissionv1beta1.Operation, marker, oldMarker *appsv1alpha1.PodMarker, subResource string) admission.Request {
	object, oldObject := toRaw(marker), toRaw(oldMarker)
	return admission.Request{
		AdmissionRequest: newAdmissionRequest(op, object, oldObject, subResource),
	}
}

func newAdmissionRequest(op admissionv1beta1.Operation, object, oldObject runtime.RawExtension, subResource string) admissionv1beta1.AdmissionRequest {
	return admissionv1beta1.AdmissionRequest{
		Resource:    metav1.GroupVersionResource{Group: corev1.SchemeGroupVersion.Group, Version: corev1.SchemeGroupVersion.Version, Resource: "pods"},
		Operation:   op,
		Object:      object,
		OldObject:   oldObject,
		SubResource: subResource,
	}
}
