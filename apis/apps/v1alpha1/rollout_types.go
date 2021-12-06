/*
Copyright 2021 The KubeVela Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TARGET",type=string,JSONPath=`.status.rolloutTargetSize`
// +kubebuilder:printcolumn:name="UPGRADED",type=string,JSONPath=`.status.upgradedReplicas`
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=`.status.upgradedReadyReplicas`
// +kubebuilder:printcolumn:name="BATCH-STATE",type=string,JSONPath=`.status.batchRollingState`
// +kubebuilder:printcolumn:name="ROLLING-STATE",type=string,JSONPath=`.status.rollingState`
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"

type BatchRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BatchReleaseSpec   `json:"spec,omitempty"`
	Status BatchReleaseStatus `json:"status,omitempty"`
}

// BatchReleaseSpec defines how to describe an update between different compRevision
type BatchReleaseSpec struct {
	// TargetRevisionName contains the name of the componentRevisionName that we need to upgrade to.
	TargetRef TargetReference `json:"targetReference"`

	// RolloutPlan is the details on how to rollout the resources
	ReleasePlan ReleasePlan `json:"releasePlan"`
}

// BatchReleaseList contains a list of BatchRelease
// +kubebuilder:object:root=true
type BatchReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BatchRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BatchRelease{}, &BatchReleaseList{})
}
