package appinfo

type AppIdentity struct {
	AppPath        string `json:"app_path"`
	BundleID       string `json:"bundle_id"`
	BundleName     string `json:"bundle_name"`
	DisplayName    string `json:"display_name"`
	ExecutableName string `json:"executable_name"`
	Version        string `json:"version"`
	Build          string `json:"build"`
}

type InspectionReport struct {
	Identity     AppIdentity `json:"identity"`
	HasSignature bool        `json:"has_signature"`
	Executable   string      `json:"executable"`
}
