/*
Copyright 2024.

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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MirrorSpec defines the desired state of Mirror
type MirrorSpec struct {
	Images []MirrorImage `json:"images"`

	// +kubebuilder:default:=5
	Parallelism int32 `json:"parallelism,omitempty"`

	Resources    *corev1.ResourceRequirements `json:"resources,omitempty"`
	SizeLimit    *resource.Quantity           `json:"sizeLimit,omitempty"`
	NodeSelector map[string]string            `json:"nodeSelector,omitempty"`

	// +kubebuilder:default:=3600
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`

	DockerConfig *corev1.SecretVolumeSource `json:"dockerConfig,omitempty"`

	Verbose bool `json:"verbose,omitempty"`

	SetSourceAnnotation bool `json:"setSourceAnnotation,omitempty"`

	HttpProxy string `json:"httpProxy,omitempty"`

	PushUseProxy bool `json:"pushUseProxy,omitempty"`
}

type MirrorImage struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	Tags      []string `json:"tags,omitempty"`
	Platforms []string `json:"platforms,omitempty"`
}

// MirrorStatus defines the observed state of Mirror
type MirrorStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Images     []ImageStatus      `json:"images,omitempty"`

	// +kubebuilder:default:=0
	Running int32 `json:"running"`
	// +kubebuilder:default:=0
	Failed int32 `json:"failed"`
	// +kubebuilder:default:=0
	Succeeded int32 `json:"succeeded"`
}

type ImageStatus struct {
	Source             string      `json:"source"`
	Target             string      `json:"target"`
	Phase              string      `json:"phase"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
	Message            string      `json:"message,omitempty"`
	Pod                string      `json:"pod,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Images",type="number",JSONPath=".status.images.length"
//+kubebuilder:printcolumn:name="Running",type="number",JSONPath=".status.running"
//+kubebuilder:printcolumn:name="Failed",type="number",JSONPath=".status.failed"
//+kubebuilder:printcolumn:name="Succeeded",type="number",JSONPath=".status.succeeded"

// Mirror is the Schema for the mirrors API
type Mirror struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MirrorSpec   `json:"spec,omitempty"`
	Status MirrorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MirrorList contains a list of Mirror
type MirrorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Mirror `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Mirror{}, &MirrorList{})
}
