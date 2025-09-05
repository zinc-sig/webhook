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

type User struct {
	ID      int    `json:"id"`
	Itsc    string `json:"itsc"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"isAdmin"`
	Courses []struct {
		CourseID int `json:"course_id"`
	} `json:"courses"`
}

const GetUserByITSC = `
query GetUserByITSC($itsc: String!) {
  users(where: {itsc: {_eq: $itsc}}) {
    id
    name
		itsc
    isAdmin: is_admin
    courses {
      course_id
    }
  }
}
`

const CreateUser = `
mutation CreateUser($itsc: String!, $name: String!) {
  insert_users_one(object: {itsc: $itsc, name: $name}) {
    id
  }
}
`
