package user

type IdentityRequest struct {
	Headers struct {
		Cookie string `json:"Cookie"`
	} `json:"headers"`
}

type IdentityResponse struct {
	XHasuraUserId         string `json:"X-Hasura-User-Id"`
	XHasuraRole           string `json:"X-Hasura-Role"`
	XHasuraAllowedCourses string `json:"X-Hasura-Allowed-Courses,omitempty"`
	XHasuraRequestedAt    string `json:"X-Hasura-Requested-At"`
}

type TokenSet struct {
	Data struct {
		IdToken string `json:"id_token"`
	} `json:"data"`
}
