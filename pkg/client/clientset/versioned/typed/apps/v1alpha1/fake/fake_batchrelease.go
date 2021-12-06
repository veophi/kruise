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

// FakeBatchReleases implements BatchReleaseInterface
type FakeBatchReleases struct {
	Fake *FakeAppsV1alpha1
	ns   string
}

var batchreleasesResource = schema.GroupVersionResource{Group: "apps.kruise.io", Version: "v1alpha1", Resource: "batchreleases"}

var batchreleasesKind = schema.GroupVersionKind{Group: "apps.kruise.io", Version: "v1alpha1", Kind: "BatchRelease"}

// Get takes name of the batchRelease, and returns the corresponding batchRelease object, and an error if there is any.
func (c *FakeBatchReleases) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.BatchRelease, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(batchreleasesResource, c.ns, name), &v1alpha1.BatchRelease{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BatchRelease), err
}

// List takes label and field selectors, and returns the list of BatchReleases that match those selectors.
func (c *FakeBatchReleases) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.BatchReleaseList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(batchreleasesResource, batchreleasesKind, c.ns, opts), &v1alpha1.BatchReleaseList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.BatchReleaseList{ListMeta: obj.(*v1alpha1.BatchReleaseList).ListMeta}
	for _, item := range obj.(*v1alpha1.BatchReleaseList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested batchReleases.
func (c *FakeBatchReleases) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(batchreleasesResource, c.ns, opts))

}

// Create takes the representation of a batchRelease and creates it.  Returns the server's representation of the batchRelease, and an error, if there is any.
func (c *FakeBatchReleases) Create(ctx context.Context, batchRelease *v1alpha1.BatchRelease, opts v1.CreateOptions) (result *v1alpha1.BatchRelease, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(batchreleasesResource, c.ns, batchRelease), &v1alpha1.BatchRelease{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BatchRelease), err
}

// Update takes the representation of a batchRelease and updates it. Returns the server's representation of the batchRelease, and an error, if there is any.
func (c *FakeBatchReleases) Update(ctx context.Context, batchRelease *v1alpha1.BatchRelease, opts v1.UpdateOptions) (result *v1alpha1.BatchRelease, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(batchreleasesResource, c.ns, batchRelease), &v1alpha1.BatchRelease{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BatchRelease), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeBatchReleases) UpdateStatus(ctx context.Context, batchRelease *v1alpha1.BatchRelease, opts v1.UpdateOptions) (*v1alpha1.BatchRelease, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(batchreleasesResource, "status", c.ns, batchRelease), &v1alpha1.BatchRelease{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BatchRelease), err
}

// Delete takes name of the batchRelease and deletes it. Returns an error if one occurs.
func (c *FakeBatchReleases) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(batchreleasesResource, c.ns, name), &v1alpha1.BatchRelease{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeBatchReleases) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(batchreleasesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.BatchReleaseList{})
	return err
}

// Patch applies the patch and returns the patched batchRelease.
func (c *FakeBatchReleases) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.BatchRelease, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(batchreleasesResource, c.ns, name, pt, data, subresources...), &v1alpha1.BatchRelease{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.BatchRelease), err
}
