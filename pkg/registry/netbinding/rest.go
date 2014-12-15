/*
Copyright 2014 Google Inc. All rights reserved.

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

package netbinding

import (
	"fmt"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/apiserver"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
)

// REST implements the RESTStorage interface for netbindings. When netbindings are written, it
// changes the network parameters of the affected pods. This information is eventually reflected
// in the actual pod after the kubelet picks it up
type REST struct {
	registry Registry
}

// NewREST creates a new REST backed by the given bindingRegistry.
func NewREST(netBindingRegistry Registry) *REST {
	return &REST{
		registry: netBindingRegistry,
	}
}

// Get the netbinding for a given PodID
func (rs *REST) Get(ctx api.Context, id string) (runtime.Object, error) {
	netbinding, err := rs.registry.GetNetBinding(ctx, id)
	return netbinding,err
}

// Delete returns an error because bindings are write-only objects.
func (rs *REST) Delete(ctx api.Context, id string) (<-chan apiserver.RESTResult, error) {
	return apiserver.MakeAsync(func() (runtime.Object, error) {
		return &api.Status{Status: api.StatusSuccess}, rs.registry.DeleteNetBinding(ctx, id)
	}), nil
}

// New returns a new binding object fit for having data unmarshalled into it.
func (*REST) New() runtime.Object {
	return &api.NetBinding{}
}

// Create attempts to make the assignment indicated by the binding it recieves.
func (b *REST) Create(ctx api.Context, obj runtime.Object) (<-chan apiserver.RESTResult, error) {
	netbinding, ok := obj.(*api.NetBinding)
	if !ok {
		return nil, fmt.Errorf("incorrect type: %#v", obj)
	}
	return apiserver.MakeAsync(func() (runtime.Object, error) {
		if err := b.registry.ApplyNetBinding(ctx, netbinding); err != nil {
			return nil, err
		}
		return &api.Status{Status: api.StatusSuccess}, nil
	}), nil
}

// Update returns an error-- this object may not be listed (as yet)
func (*REST) List(ctx api.Context, label, field labels.Selector) (runtime.Object, error) {
	return nil, fmt.Errorf("NetBindings may not be listed.")
}

// Update returns an error-- this object may not be updated (as yet)
func (b *REST) Update(ctx api.Context, obj runtime.Object) (<-chan apiserver.RESTResult, error) {
	return nil, fmt.Errorf("NetBindings may not be changed.")
}
