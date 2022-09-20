/*
Copyright 2022.

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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
)

type DeploymentSpecApplyConfiguration appsv1apply.DeploymentSpecApplyConfiguration
type ServiceSpecApplyConfiguration corev1apply.ServiceSpecApplyConfiguration

func (c *DeploymentSpecApplyConfiguration) DeepCopy() *DeploymentSpecApplyConfiguration {
	out := new(DeploymentSpecApplyConfiguration)
	bytes, err := json.Marshal(c)
	if err != nil {
		panic("Failed to marshal")
	}
	err = json.Unmarshal(bytes, out)
	if err != nil {
		panic("Failed to unmarshal")
	}
	return out
}

func (c *ServiceSpecApplyConfiguration) DeepCopy() *ServiceSpecApplyConfiguration {
	out := new(ServiceSpecApplyConfiguration)
	bytes, err := json.Marshal(c)
	if err != nil {
		panic("Failed to marshal")
	}
	err = json.Unmarshal(bytes, out)
	if err != nil {
		panic("Failed to unmarshal")
	}
	return out
}

// SSANginxSpec defines the desired state of SSANginx
type SSANginxSpec struct {
	DeploymentName string                            `json:"deploymentName"`
	DeploymentSpec *DeploymentSpecApplyConfiguration `json:"deploymentSpec"`
	ConfigMapName  string                            `json:"configMapName"`
	ConfigMapData  map[string]string                 `json:"configMapData,omitempty"`
	ServiceName    string                            `json:"serviceName"`
	ServiceSpec    *ServiceSpecApplyConfiguration    `json:"serviceSpec"`
}

// SSANginxStatus defines the observed state of SSANginx
type SSANginxStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// SSANginx is the Schema for the ssanginxes API
type SSANginx struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SSANginxSpec   `json:"spec,omitempty"`
	Status SSANginxStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SSANginxList contains a list of SSANginx
type SSANginxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SSANginx `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SSANginx{}, &SSANginxList{})
}
