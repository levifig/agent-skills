package vnextflowcontract

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestFlowJSONContractsRejectDuplicateObjectKeysRecursively(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name      string
		path      string
		body      string
		newTarget func() any
	}{
		{name: "flow top level", path: flowManifestPath, body: duplicateBefore(validFlowManifest, `"schema": "loaf-flow/v1"`, `"schema": "duplicate"`), newTarget: func() any { return &flowManifest{} }},
		{name: "flow nested", path: flowManifestPath, body: duplicateBefore(validFlowManifest, `"shared_work": "tracker"`, `"shared_work": "local"`), newTarget: func() any { return &flowManifest{} }},
		{name: "common tracker top level", path: projectManagementContractPath, body: duplicateBefore(validProjectManagementContract, `"id": "project-management/v1"`, `"id": "duplicate"`), newTarget: func() any { return &projectManagementContract{} }},
		{name: "common tracker nested", path: projectManagementContractPath, body: duplicateBefore(validProjectManagementContract, `"read_before_write": "required"`, `"read_before_write": "optional"`), newTarget: func() any { return &projectManagementContract{} }},
		{name: "linear top level", path: linearCapabilitiesPath, body: duplicateBefore(validLinearCapabilities, `"provider": "linear"`, `"provider": "other"`), newTarget: func() any { return &providerCapabilities{} }},
		{name: "linear nested", path: linearCapabilitiesPath, body: duplicateBefore(validLinearCapabilities, `"native_semantic": "harness.exposed-linear-connection"`, `"native_semantic": "other"`), newTarget: func() any { return &providerCapabilities{} }},
		{name: "profile top level", path: projectManagerContractPath, body: duplicateBefore(validProjectManagerContract, `"execution": "optional"`, `"execution": "required"`), newTarget: func() any { return &projectManagerProfileContract{} }},
		{name: "profile nested", path: projectManagerContractPath, body: duplicateBefore(validProjectManagerContract, `"contract_id": "project-management/v1"`, `"contract_id": "other"`), newTarget: func() any { return &projectManagerProfileContract{} }},
	}

	for _, fixtureCase := range fixtures {
		fixtureCase := fixtureCase
		t.Run(fixtureCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := fstest.MapFS{fixtureCase.path: &fstest.MapFile{Data: []byte(fixtureCase.body)}}
			first := decodeStrictJSON(fixture, fixtureCase.path, fixtureCase.newTarget())
			second := decodeStrictJSON(fixture, fixtureCase.path, fixtureCase.newTarget())
			if first == nil || second == nil {
				t.Fatalf("duplicate object key accepted: first=%v second=%v", first, second)
			}
			if first.Error() != second.Error() || !strings.Contains(first.Error(), "duplicate JSON object key") {
				t.Fatalf("duplicate-key error is not deterministic: first=%v second=%v", first, second)
			}
		})
	}
}

func TestFlowJSONContractsRetainTrailingValueRejection(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name string
		body string
		want string
	}{
		{name: "second value", body: validFlowManifest + "\n{}", want: "multiple JSON values"},
		{name: "trailing syntax", body: validFlowManifest + "\ninvalid", want: "trailing JSON"},
	}

	for _, fixtureCase := range fixtures {
		fixtureCase := fixtureCase
		t.Run(fixtureCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := fstest.MapFS{flowManifestPath: &fstest.MapFile{Data: []byte(fixtureCase.body)}}
			err := decodeStrictJSON(fixture, flowManifestPath, &flowManifest{})
			if err == nil || !strings.Contains(err.Error(), fixtureCase.want) {
				t.Fatalf("decode error = %v, want %q", err, fixtureCase.want)
			}
		})
	}
}

func duplicateBefore(body, original, duplicate string) string {
	return strings.Replace(body, original, duplicate+", "+original, 1)
}
