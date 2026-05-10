package gcp

import (
	"strings"

	"cloudcmder.com/internal/inventory"
)

// vertexAssetSubtype maps CAI aiplatform.googleapis.com/* asset type suffixes
// to the VertexDetail.Subtype label shown in the TUI. New asset types added by
// Google automatically fall through to "Other" so rows still surface without a
// code change.
var vertexAssetSubtype = map[string]string{
	"aiplatform.googleapis.com/BatchPredictionJob":          "BatchPredictionJob",
	"aiplatform.googleapis.com/CachedContent":               "CachedContent",
	"aiplatform.googleapis.com/CustomJob":                   "CustomJob",
	"aiplatform.googleapis.com/Dataset":                     "Dataset",
	"aiplatform.googleapis.com/DeploymentResourcePool":      "DeploymentResourcePool",
	"aiplatform.googleapis.com/Endpoint":                    "Endpoint",
	"aiplatform.googleapis.com/Featurestore":                "Featurestore",
	"aiplatform.googleapis.com/FeatureGroup":                "FeatureGroup",
	"aiplatform.googleapis.com/FeatureOnlineStore":          "FeatureOnlineStore",
	"aiplatform.googleapis.com/HyperparameterTuningJob":     "HyperparameterTuningJob",
	"aiplatform.googleapis.com/Index":                       "Index",
	"aiplatform.googleapis.com/IndexEndpoint":               "IndexEndpoint",
	"aiplatform.googleapis.com/MetadataStore":               "MetadataStore",
	"aiplatform.googleapis.com/Model":                       "Model",
	"aiplatform.googleapis.com/ModelDeploymentMonitoringJob": "ModelDeploymentMonitoringJob",
	"aiplatform.googleapis.com/NotebookRuntime":             "NotebookRuntime",
	"aiplatform.googleapis.com/NotebookRuntimeTemplate":     "NotebookRuntimeTemplate",
	"aiplatform.googleapis.com/PipelineJob":                 "PipelineJob",
	"aiplatform.googleapis.com/ReasoningEngine":             "ReasoningEngine",
	"aiplatform.googleapis.com/Schedule":                    "Schedule",
	"aiplatform.googleapis.com/SpecialistPool":              "SpecialistPool",
	"aiplatform.googleapis.com/Tensorboard":                 "Tensorboard",
	"aiplatform.googleapis.com/TrainingPipeline":            "TrainingPipeline",
	"aiplatform.googleapis.com/TuningJob":                   "TuningJob",
}

// vertexSubtypeFromAssetType returns the VertexDetail.Subtype label for the
// given CAI asset type. Non-Vertex types and unknown Vertex types both return
// "Other".
func vertexSubtypeFromAssetType(at string) string {
	if sub, ok := vertexAssetSubtype[at]; ok {
		return sub
	}
	// Catch future aiplatform.* additions without a map entry.
	if strings.HasPrefix(at, "aiplatform.googleapis.com/") {
		return "Other"
	}
	return "Other"
}

// vertexDetailFromAssetType returns a *VertexDetail for any
// aiplatform.googleapis.com/* CAI asset type, or nil for all other asset types.
// Called from translateResult so KindVertexAI stubs carry the Subtype label
// without a Phase-2 enricher.
func vertexDetailFromAssetType(at string) *inventory.VertexDetail {
	if !strings.HasPrefix(at, "aiplatform.googleapis.com/") {
		return nil
	}
	return &inventory.VertexDetail{
		Subtype: vertexSubtypeFromAssetType(at),
	}
}
