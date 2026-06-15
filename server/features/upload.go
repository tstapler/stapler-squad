package features

import "github.com/tstapler/stapler-squad/server/featureregistry"

var UploadFile = featureregistry.Feature{
	ID:          "upload-file",
	Title:       "Upload File",
	Description: "Handles file uploads via HTTP POST to the upload endpoint.",
	RPCIDs:      []string{"upload:file"},
	Status:      featureregistry.StatusExperimental,
	Since:       "1.0.0",
}

var UploadImage = featureregistry.Feature{
	ID:          "upload-image",
	Title:       "Upload Image",
	Description: "Handles image uploads via HTTP POST, supporting JPEG, PNG, and WebP formats.",
	RPCIDs:      []string{"upload:image"},
	Status:      featureregistry.StatusStable,
	Since:       "1.0.0",
}

func init() {
	featureregistry.Register(UploadFile)
	featureregistry.Register(UploadImage)
}
