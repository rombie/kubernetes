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

package generic

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/runtime"
)

type Ignored struct {
	ID string
}

type IgnoredList struct {
	Items []Ignored
}

func (*Ignored) IsAnAPIObject()     {}
func (*IgnoredList) IsAnAPIObject() {}

func TestSelectionPredicate(t *testing.T) {
	table := map[string]struct {
		labelSelector, fieldSelector string
		labels                       labels.Set
		fields                       fields.Set
		err                          error
		shouldMatch                  bool
		matchSingleKey               string
	}{
		"A": {
			labelSelector: "name=foo",
			fieldSelector: "uid=12345",
			labels:        labels.Set{"name": "foo"},
			fields:        fields.Set{"uid": "12345"},
			shouldMatch:   true,
		},
		"B": {
			labelSelector: "name=foo",
			fieldSelector: "uid=12345",
			labels:        labels.Set{"name": "foo"},
			fields:        fields.Set{},
			shouldMatch:   false,
		},
		"C": {
			labelSelector: "name=foo",
			fieldSelector: "uid=12345",
			labels:        labels.Set{},
			fields:        fields.Set{"uid": "12345"},
			shouldMatch:   false,
		},
		"D": {
			fieldSelector:  "metadata.name=12345",
			labels:         labels.Set{},
			fields:         fields.Set{"metadata.name": "12345"},
			shouldMatch:    true,
			matchSingleKey: "12345",
		},
		"error": {
			labelSelector: "name=foo",
			fieldSelector: "uid=12345",
			err:           errors.New("maybe this is a 'wrong object type' error"),
			shouldMatch:   false,
		},
	}

	for name, item := range table {
		parsedLabel, err := labels.Parse(item.labelSelector)
		if err != nil {
			panic(err)
		}
		parsedField, err := fields.ParseSelector(item.fieldSelector)
		if err != nil {
			panic(err)
		}
		sp := &SelectionPredicate{
			Label: parsedLabel,
			Field: parsedField,
			GetAttrs: func(runtime.Object) (label labels.Set, field fields.Set, err error) {
				return item.labels, item.fields, item.err
			},
		}
		got, err := sp.Matches(&Ignored{})
		if e, a := item.err, err; e != a {
			t.Errorf("%v: expected %v, got %v", name, e, a)
			continue
		}
		if e, a := item.shouldMatch, got; e != a {
			t.Errorf("%v: expected %v, got %v", name, e, a)
		}
		if key := item.matchSingleKey; key != "" {
			got, ok := sp.MatchesSingle()
			if !ok {
				t.Errorf("%v: expected single match", name)
			}
			if e, a := key, got; e != a {
				t.Errorf("%v: expected %v, got %v", name, e, a)
			}
		}
	}
}

func TestFilterList(t *testing.T) {
	try := &IgnoredList{
		Items: []Ignored{
			{"foo"},
			{"bar"},
			{"baz"},
			{"qux"},
			{"zot"},
		},
	}
	expect := &IgnoredList{
		Items: []Ignored{
			{"bar"},
			{"baz"},
		},
	}

	m := MatcherFunc(func(obj runtime.Object) (bool, error) {
		i, ok := obj.(*Ignored)
		if !ok {
			return false, errors.New("wrong type")
		}
		return i.ID[0] == 'b', nil
	})
	if _, matchesSingleObject := m.MatchesSingle(); matchesSingleObject {
		t.Errorf("matcher unexpectedly matches only a single object.")
	}

	got, err := FilterList(try, m, nil)
	if err != nil {
		t.Fatalf("Unexpected error %v", err)
	}

	if e, a := expect, got; !reflect.DeepEqual(e, a) {
		t.Errorf("Expected %#v, got %#v", e, a)
	}
}

func TestSingleMatch(t *testing.T) {
	m := MatchOnKey("pod-name-here", func(obj runtime.Object) (bool, error) { return true, nil })
	got, ok := m.MatchesSingle()
	if !ok {
		t.Errorf("Expected MatchesSingle to return true")
	}
	if e, a := "pod-name-here", got; e != a {
		t.Errorf("Expected %#v, got %#v", e, a)
	}
}
