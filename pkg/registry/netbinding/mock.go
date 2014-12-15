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
	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
)

// MockRegistry can be used for testing.
type MockRegistry struct {
	OnApplyNetBinding func(netbinding *api.NetBinding) error
	OnGetNetBinding func(ctx api.Context, podID string) (*api.NetBinding,error)
	OnDeleteNetBinding func(ctx api.Context, podID string) error
}

func (mr MockRegistry) ApplyNetBinding(ctx api.Context, netbinding *api.NetBinding) error {
	return mr.OnApplyNetBinding(netbinding)
}

func (mr MockRegistry) GetNetBinding(ctx api.Context, podID string) (*api.NetBinding, error) {
	return mr.OnGetNetBinding(ctx, podID)
}

func (mr MockRegistry) DeleteNetBinding(ctx api.Context, podID string) error {
	return mr.OnDeleteNetBinding(ctx, podID)
}
