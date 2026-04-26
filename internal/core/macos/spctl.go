package macos

import "context"

type AssessResult struct {
	Accepted bool   `json:"accepted"`
	Output   string `json:"output,omitempty"`
}

func Assess(ctx context.Context, ex Execer, appPath string) (*AssessResult, error) {
	_, stderr, code, err := ex.Run(ctx, "spctl", "--assess", "--type", "execute", "--verbose", appPath)
	if err != nil {
		return nil, err
	}
	return &AssessResult{
		Accepted: code == 0,
		Output:   string(stderr),
	}, nil
}
