// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package operator

import (
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/config"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestRuleEvaluatorConfigUnmarshal(t *testing.T) {
	code := `
rule_files:
    - /etc/rules/*.yaml
google_cloud:
    export:
        compression: gzip
        credentials: credentials1.json
    query:
        project_id: abc123
        generator_url: http://example.com/
        credentials: credentials2.json
`
	out := RuleEvaluatorConfig{}
	if err := yaml.Unmarshal([]byte(code), &out); err != nil {
		t.Fatal(err)
	}

	expected := RuleEvaluatorConfig{
		Config: config.DefaultConfig,
		GoogleCloud: GoogleCloudConfig{
			Query: &GoogleCloudQueryConfig{
				ProjectID:       "abc123",
				GeneratorURL:    "http://example.com/",
				CredentialsFile: "credentials2.json",
			},
			Export: &GoogleCloudExportConfig{
				Compression:     ptr.To(string(monitoringv1.CompressionGzip)),
				CredentialsFile: ptr.To("credentials1.json"),
			},
		},
	}
	expected.RuleFiles = []string{"/etc/rules/*.yaml"}
	if diff := cmp.Diff(expected, out); diff != "" {
		t.Fatalf("unexpected config from marshaling (-want, +got): %s", diff)
	}

	// Ensure we can also marshal. Reuse the same object.
	outBytes, err := yaml.Marshal(&out)
	if err != nil {
		t.Fatal(err)
	}
	out = RuleEvaluatorConfig{}
	if err := yaml.Unmarshal(outBytes, &out); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(expected, out); diff != "" {
		t.Fatalf("unexpected config after marshaling (-want, +got): %s", diff)
	}
}

func TestEnsureOperatorConfig(t *testing.T) {
	logger := logr.Discard()
	operatorOpts := Options{
		ProjectID: "test-project",
		Location:  "us-central1-c",
		Cluster:   "test-cluster",
	}
	defaultObjectMeta := v1.ObjectMeta{
		Namespace: DefaultPublicNamespace,
		Name:      NameOperatorConfig,
	}
	defaultOperatorConfig := monitoringv1.OperatorConfig{
		ObjectMeta: defaultObjectMeta,
		Collection: monitoringv1.CollectionSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
			},
		},
		Rules: monitoringv1.RuleEvaluatorSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
			},
		},
	}
	defaultOperatorConfigWithExtraLabels := monitoringv1.OperatorConfig{
		ObjectMeta: defaultObjectMeta,
		Collection: monitoringv1.CollectionSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
				"foo":               "bar",
			},
		},
		Rules: monitoringv1.RuleEvaluatorSpec{
			ExternalLabels: map[string]string{
				export.KeyProjectID: operatorOpts.ProjectID,
				export.KeyLocation:  operatorOpts.Location,
				export.KeyCluster:   operatorOpts.Cluster,
				"abc":               "xyz",
			},
		},
	}

	testCases := []struct {
		desc     string
		existing *monitoringv1.OperatorConfig
		expected *monitoringv1.OperatorConfig
	}{
		{
			desc: "empty",
			existing: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
			},
			expected: &defaultOperatorConfig,
		},
		{
			desc:     "does not exist",
			existing: nil,
			expected: &defaultOperatorConfig,
		},
		{
			desc:     "matches config",
			existing: &defaultOperatorConfigWithExtraLabels,
			expected: &defaultOperatorConfigWithExtraLabels,
		},
		{
			desc: "missing project",
			existing: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
				Collection: monitoringv1.CollectionSpec{
					ExternalLabels: map[string]string{
						export.KeyLocation: operatorOpts.Location,
						export.KeyCluster:  operatorOpts.Cluster,
						"foo":              "bar",
					},
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					ExternalLabels: map[string]string{
						export.KeyLocation: operatorOpts.Location,
						export.KeyCluster:  operatorOpts.Cluster,
						"abc":              "xyz",
					},
				},
			},
			expected: &defaultOperatorConfigWithExtraLabels,
		},
		{
			desc: "override project",
			existing: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
				Collection: monitoringv1.CollectionSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"foo":               "bar",
					},
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"abc":               "xyz",
					},
				},
			},
			expected: &monitoringv1.OperatorConfig{
				ObjectMeta: defaultObjectMeta,
				Collection: monitoringv1.CollectionSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"foo":               "bar",
					},
				},
				Rules: monitoringv1.RuleEvaluatorSpec{
					ExternalLabels: map[string]string{
						export.KeyProjectID: "project-other",
						export.KeyLocation:  operatorOpts.Location,
						export.KeyCluster:   operatorOpts.Cluster,
						"abc":               "xyz",
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			clientBuilder := newFakeClientBuilder()
			if tc.existing != nil {
				clientBuilder = clientBuilder.WithObjects(tc.existing.DeepCopy())
			}
			kubeClient := clientBuilder.Build()

			reconciler := newOperatorConfigReconciler(kubeClient, operatorOpts)
			operatorConfig, err := reconciler.ensureOperatorConfig(t.Context(), logger, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: DefaultPublicNamespace,
					Name:      NameOperatorConfig,
				},
			})
			if err != nil {
				t.Fatalf("ensure operator config: %s", err)
			}

			// Normalize before comparisons.
			operatorConfig.ResourceVersion = ""

			if diff := cmp.Diff(operatorConfig, tc.expected); diff != "" {
				t.Fatalf("unexpected operator config: %s", diff)
			}

			// Make sure the object is updated in case of defaulting.
			if tc.existing != nil {
				existingLatest := monitoringv1.OperatorConfig{}
				if err := kubeClient.Get(t.Context(), client.ObjectKeyFromObject(tc.existing), &existingLatest); err != nil {
					t.Fatalf("get operator config: %s", err)
				}

				// Normalize before comparisons.
				existingLatest.ResourceVersion = ""

				if diff := cmp.Diff(&existingLatest, tc.expected); diff != "" {
					t.Fatalf("operator config expected update: %s", diff)
				}
			}
		})
	}
}
