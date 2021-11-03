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

package fake

import (
	"context"

	v1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakePodMarkers implements PodMarkerInterface
type FakePodMarkers struct {
	Fake *FakeAppsV1alpha1
	ns   string
}

var podmarkersResource = schema.GroupVersionResource{Group: "apps.kruise.io", Version: "v1alpha1", Resource: "podmarkers"}

var podmarkersKind = schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "PodMarker"}

// Get takes name of the podMarker, and returns the corresponding podMarker object, and an error if there is any.
func (c *FakePodMarkers) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.PodMarker, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(podmarkersResource, c.ns, name), &v1alpha1.PodMarker{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PodMarker), err
}

// List takes label and field selectors, and returns the list of PodMarkers that match those selectors.
func (c *FakePodMarkers) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.PodMarkerList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(podmarkersResource, podmarkersKind, c.ns, opts), &v1alpha1.PodMarkerList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.PodMarkerList{ListMeta: obj.(*v1alpha1.PodMarkerList).ListMeta}
	for _, item := range obj.(*v1alpha1.PodMarkerList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested podMarkers.
func (c *FakePodMarkers) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(podmarkersResource, c.ns, opts))

}

// Create takes the representation of a podMarker and creates it.  Returns the server's representation of the podMarker, and an error, if there is any.
func (c *FakePodMarkers) Create(ctx context.Context, podMarker *v1alpha1.PodMarker, opts v1.CreateOptions) (result *v1alpha1.PodMarker, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(podmarkersResource, c.ns, podMarker), &v1alpha1.PodMarker{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PodMarker), err
}

// Update takes the representation of a podMarker and updates it. Returns the server's representation of the podMarker, and an error, if there is any.
func (c *FakePodMarkers) Update(ctx context.Context, podMarker *v1alpha1.PodMarker, opts v1.UpdateOptions) (result *v1alpha1.PodMarker, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(podmarkersResource, c.ns, podMarker), &v1alpha1.PodMarker{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PodMarker), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakePodMarkers) UpdateStatus(ctx context.Context, podMarker *v1alpha1.PodMarker, opts v1.UpdateOptions) (*v1alpha1.PodMarker, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(podmarkersResource, "status", c.ns, podMarker), &v1alpha1.PodMarker{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PodMarker), err
}

// Delete takes name of the podMarker and deletes it. Returns an error if one occurs.
func (c *FakePodMarkers) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(podmarkersResource, c.ns, name), &v1alpha1.PodMarker{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakePodMarkers) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(podmarkersResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.PodMarkerList{})
	return err
}

// Patch applies the patch and returns the patched podMarker.
func (c *FakePodMarkers) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.PodMarker, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(podmarkersResource, c.ns, name, pt, data, subresources...), &v1alpha1.PodMarker{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PodMarker), err
}