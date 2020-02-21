/*
Copyright 2020 Flavio Castelli <fcastelli@suse.com>.

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

// FreshContainerMonitorSpec defines the desired state of FreshContainerMonitor
type FreshContainerMonitorSpec struct {
	// FreshContainerServerURL is the full url to an instance of FreshContainer server
	FreshContainerServerURL string `json:"fresh_container_server_url,omitempty"`

	// CheckIntervalMinutes is the amount of minutes between checks
	// +kubebuilder:validation:Default=10
	CheckIntervalMinutes int `json:"check_interval_minutes"`
}

// FreshContainerMonitorStatus defines the observed state of FreshContainerMonitor
type FreshContainerMonitorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// FreshContainerMonitor is the Schema for the freshcontainermonitors API
type FreshContainerMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FreshContainerMonitorSpec   `json:"spec,omitempty"`
	Status FreshContainerMonitorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FreshContainerMonitorList contains a list of FreshContainerMonitor
type FreshContainerMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FreshContainerMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FreshContainerMonitor{}, &FreshContainerMonitorList{})
}
