package appinfo

type AppIdentity struct {
	AppPath        string
	BundleID       string
	BundleName     string
	DisplayName    string
	ExecutableName string
	Version        string
	Build          string
}

type InspectionReport struct {
	Identity     AppIdentity
	HasSignature bool
	Executable   string
}
