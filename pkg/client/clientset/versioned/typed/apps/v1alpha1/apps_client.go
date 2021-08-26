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
// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	"github.com/openkruise/kruise/pkg/client/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

type AppsV1alpha1Interface interface {
	RESTClient() rest.Interface
	AdvancedCronJobsGetter
	BroadcastJobsGetter
	CloneSetsGetter
	ContainerRecreateRequestsGetter
	DaemonSetsGetter
	EphemeralJobsGetter
	ImagePullJobsGetter
	NodeImagesGetter
	PodMarkersGetter
	ResourceDistributionsGetter
	SidecarSetsGetter
	StatefulSetsGetter
	UnitedDeploymentsGetter
	WorkloadSpreadsGetter
}

// AppsV1alpha1Client is used to interact with features provided by the apps.kruise.io group.
type AppsV1alpha1Client struct {
	restClient rest.Interface
}

func (c *AppsV1alpha1Client) AdvancedCronJobs(namespace string) AdvancedCronJobInterface {
	return newAdvancedCronJobs(c, namespace)
}

func (c *AppsV1alpha1Client) BroadcastJobs(namespace string) BroadcastJobInterface {
	return newBroadcastJobs(c, namespace)
}

func (c *AppsV1alpha1Client) CloneSets(namespace string) CloneSetInterface {
	return newCloneSets(c, namespace)
}

func (c *AppsV1alpha1Client) ContainerRecreateRequests(namespace string) ContainerRecreateRequestInterface {
	return newContainerRecreateRequests(c, namespace)
}

func (c *AppsV1alpha1Client) DaemonSets(namespace string) DaemonSetInterface {
	return newDaemonSets(c, namespace)
}

func (c *AppsV1alpha1Client) EphemeralJobs(namespace string) EphemeralJobInterface {
	return newEphemeralJobs(c, namespace)
}

func (c *AppsV1alpha1Client) ImagePullJobs(namespace string) ImagePullJobInterface {
	return newImagePullJobs(c, namespace)
}

func (c *AppsV1alpha1Client) NodeImages() NodeImageInterface {
	return newNodeImages(c)
}

func (c *AppsV1alpha1Client) PodMarkers(namespace string) PodMarkerInterface {
	return newPodMarkers(c, namespace)
}

func (c *AppsV1alpha1Client) ResourceDistributions() ResourceDistributionInterface {
	return newResourceDistributions(c)
}

func (c *AppsV1alpha1Client) SidecarSets() SidecarSetInterface {
	return newSidecarSets(c)
}

func (c *AppsV1alpha1Client) StatefulSets(namespace string) StatefulSetInterface {
	return newStatefulSets(c, namespace)
}

func (c *AppsV1alpha1Client) UnitedDeployments(namespace string) UnitedDeploymentInterface {
	return newUnitedDeployments(c, namespace)
}

func (c *AppsV1alpha1Client) WorkloadSpreads(namespace string) WorkloadSpreadInterface {
	return newWorkloadSpreads(c, namespace)
}

// NewForConfig creates a new AppsV1alpha1Client for the given config.
func NewForConfig(c *rest.Config) (*AppsV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &AppsV1alpha1Client{client}, nil
}

// NewForConfigOrDie creates a new AppsV1alpha1Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *AppsV1alpha1Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new AppsV1alpha1Client for the given RESTClient.
func New(c rest.Interface) *AppsV1alpha1Client {
	return &AppsV1alpha1Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1alpha1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *AppsV1alpha1Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
