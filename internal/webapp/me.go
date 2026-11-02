package webapp

import "context"

type meOutput struct {
	Body struct {
		Language  string `doc:"The language to show the app in" enum:"en,uk"      json:"language"`
		FirstName string `doc:"The user's first name"           json:"first_name"`
	}
}

func (o *Operations) me(ctx context.Context, _ *struct{}) (*meOutput, error) {
	c, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	out := &meOutput{}
	out.Body.Language = c.Lang
	out.Body.FirstName = c.User.FirstName

	return out, nil
}
