package exitcodes

const (
	OK                     = 0
	GeneralError           = 1
	InvalidInput           = 2
	UnsupportedEnvironment = 3
	CopyFailed             = 4
	PlistMutationFailed    = 5
	SigningFailed          = 6
	VerificationFailed     = 7
	LaunchTestFailed       = 8
	InspectionFailed       = 9
)
