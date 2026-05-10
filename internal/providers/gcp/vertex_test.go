package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVertexSubtypeFromAssetType(t *testing.T) {
	cases := []struct {
		name    string
		assetType string
		want    string
	}{
		{"Endpoint", "aiplatform.googleapis.com/Endpoint", "Endpoint"},
		{"Model", "aiplatform.googleapis.com/Model", "Model"},
		{"Dataset", "aiplatform.googleapis.com/Dataset", "Dataset"},
		{"Index", "aiplatform.googleapis.com/Index", "Index"},
		{"IndexEndpoint", "aiplatform.googleapis.com/IndexEndpoint", "IndexEndpoint"},
		{"PipelineJob", "aiplatform.googleapis.com/PipelineJob", "PipelineJob"},
		{"TrainingPipeline", "aiplatform.googleapis.com/TrainingPipeline", "TrainingPipeline"},
		{"Featurestore", "aiplatform.googleapis.com/Featurestore", "Featurestore"},
		{"FeatureGroup", "aiplatform.googleapis.com/FeatureGroup", "FeatureGroup"},
		{"FeatureOnlineStore", "aiplatform.googleapis.com/FeatureOnlineStore", "FeatureOnlineStore"},
		{"NotebookRuntime", "aiplatform.googleapis.com/NotebookRuntime", "NotebookRuntime"},
		{"NotebookRuntimeTemplate", "aiplatform.googleapis.com/NotebookRuntimeTemplate", "NotebookRuntimeTemplate"},
		{"MetadataStore", "aiplatform.googleapis.com/MetadataStore", "MetadataStore"},
		{"Schedule", "aiplatform.googleapis.com/Schedule", "Schedule"},
		{"BatchPredictionJob", "aiplatform.googleapis.com/BatchPredictionJob", "BatchPredictionJob"},
		{"CustomJob", "aiplatform.googleapis.com/CustomJob", "CustomJob"},
		{"HyperparameterTuningJob", "aiplatform.googleapis.com/HyperparameterTuningJob", "HyperparameterTuningJob"},
		{"ModelDeploymentMonitoringJob", "aiplatform.googleapis.com/ModelDeploymentMonitoringJob", "ModelDeploymentMonitoringJob"},
		{"Tensorboard", "aiplatform.googleapis.com/Tensorboard", "Tensorboard"},
		{"TuningJob", "aiplatform.googleapis.com/TuningJob", "TuningJob"},
		{"ReasoningEngine", "aiplatform.googleapis.com/ReasoningEngine", "ReasoningEngine"},
		{"CachedContent", "aiplatform.googleapis.com/CachedContent", "CachedContent"},
		{"DeploymentResourcePool", "aiplatform.googleapis.com/DeploymentResourcePool", "DeploymentResourcePool"},
		{"SpecialistPool", "aiplatform.googleapis.com/SpecialistPool", "SpecialistPool"},
		{"unknown vertex type", "aiplatform.googleapis.com/FutureResource", "Other"},
		{"non-vertex type", "compute.googleapis.com/Instance", "Other"},
		{"empty", "", "Other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, vertexSubtypeFromAssetType(tc.assetType))
		})
	}
}

func TestVertexDetailFromAssetType(t *testing.T) {
	cases := []struct {
		name      string
		assetType string
		wantNil   bool
		wantSub   string
	}{
		{"Endpoint returns detail", "aiplatform.googleapis.com/Endpoint", false, "Endpoint"},
		{"Model returns detail", "aiplatform.googleapis.com/Model", false, "Model"},
		{"unknown vertex returns Other", "aiplatform.googleapis.com/FutureResource", false, "Other"},
		{"compute type returns nil", "compute.googleapis.com/Instance", true, ""},
		{"empty returns nil", "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := vertexDetailFromAssetType(tc.assetType)
			if tc.wantNil {
				assert.Nil(t, got)
				return
			}
			assert.NotNil(t, got)
			assert.Equal(t, tc.wantSub, got.Subtype)
		})
	}
}
