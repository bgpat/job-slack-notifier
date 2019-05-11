/*
Copyright 2019 bgpat.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// JobNotifierSpec defines the desired state of JobNotifier
type JobNotifierSpec struct {
	// A label query to watch job status changes.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Specifies the channel list to send the notification.
	Channels []string `json:"channels,omitempty"`

	// Specifies the mentioned user for the notification.
	MentionTo []string `json:"mention_to,omitempty"`
}

// JobNotifierStatus defines the observed state of JobNotifier
type JobNotifierStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// JobNotifier is the Schema for the jobnotifiers API
// +k8s:openapi-gen=true
type JobNotifier struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JobNotifierSpec   `json:"spec,omitempty"`
	Status JobNotifierStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// JobNotifierList contains a list of JobNotifier
type JobNotifierList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JobNotifier `json:"items"`
}

func init() {
	SchemeBuilder.Register(&JobNotifier{}, &JobNotifierList{})
}
