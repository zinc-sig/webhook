package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/machinebox/graphql"
)

var ctx = context.Background()

const GetUserByITSC = `
query GetUserByITSC($itsc: String!) {
  users(where: {itsc: {_eq: $itsc}}) {
    id
    name
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

type Handler struct {
	GraphQLClient GraphQLClient
	RedisClient   redis.Cmdable
}

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

type CookieData struct {
	Data struct {
		IdToken string `json:"id_token"`
	} `json:"data"`
}

type User struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"isAdmin"`
	Courses []struct {
		CourseID int `json:"course_id"`
	} `json:"courses"`
}

func (h *Handler) Identity(c echo.Context) error {
	var req IdentityRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	appSession, err := getCookie(req.Headers.Cookie, "appSession")
	if err != nil {
		return c.String(http.StatusUnauthorized, "Unauthorized")
	}

	hmac := hmac.New(sha1.New, []byte(os.Getenv("SESSION_SECRET")))
	hmac.Write([]byte(appSession))
	sid := base64.StdEncoding.EncodeToString(hmac.Sum(nil))

	cookie, err := h.RedisClient.Get(ctx, sid).Result()
	if err != nil {
		return c.String(http.StatusUnauthorized, "Could not find request session with auth credentials")
	}

	var cookieData CookieData
	if err := json.Unmarshal([]byte(cookie), &cookieData); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to parse cookie")
	}

	token, err := verifyToken(cookieData.Data.IdToken)
	if err != nil {
		return c.String(http.StatusUnauthorized, "Invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return c.String(http.StatusUnauthorized, "Invalid token")
	}

	email, ok := claims["email"].(string)
	if !ok {
		return c.String(http.StatusUnauthorized, "Invalid token")
	}
	itsc := strings.Split(email, "@")[0]
	name, _ := claims["name"].(string)

	user, err := h.getUser(itsc, name)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to get user")
	}

	allowedCourses := ""
	if !user.IsAdmin {
		var courseIDs []string
		for _, course := range user.Courses {
			courseIDs = append(courseIDs, fmt.Sprintf("%d", course.CourseID))
		}
		allowedCourses = fmt.Sprintf("{%s}", strings.Join(courseIDs, ","))
	}

	role := "user"
	if user.IsAdmin {
		role = "admin"
	}

	resp := IdentityResponse{
		XHasuraUserId:         itsc,
		XHasuraRole:           role,
		XHasuraAllowedCourses: allowedCourses,
		XHasuraRequestedAt:    time.Now().Format(time.RFC3339),
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) getUser(itsc, name string) (*User, error) {
	req := graphql.NewRequest(GetUserByITSC)
	req.Var("itsc", itsc)

	var resp struct {
		Users []User `json:"users"`
	}

	if err := h.GraphQLClient.Run(ctx, req, &resp); err != nil {
		return nil, err
	}

	if len(resp.Users) > 0 {
		return &resp.Users[0], nil
	}

	// Create user if not found
	req = graphql.NewRequest(CreateUser)
	req.Var("itsc", itsc)
	req.Var("name", name)

	var createResp struct {
		InsertUsersOne struct {
			ID int `json:"id"`
		} `json:"insert_users_one"`
	}

	if err := h.GraphQLClient.Run(ctx, req, &createResp); err != nil {
		return nil, err
	}

	return &User{ID: createResp.InsertUsersOne.ID, Name: name}, nil
}

func getCookie(cookieHeader, cookieName string) (string, error) {
	header := http.Header{}
	header.Add("Cookie", cookieHeader)
	request := http.Request{Header: header}
	cookie, err := request.Cookie(cookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

var JwkFetch = jwk.Fetch

func verifyToken(tokenString string) (*jwt.Token, error) {
	keySet, err := JwkFetch(context.Background(), "https://login.microsoftonline.com/common/discovery/v2.0/keys")
	if err != nil {
		return nil, err
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("kid header not found")
		}

		keys, ok := keySet.LookupKeyID(kid)
		if !ok {
			return nil, errors.New("key not found")
		}

		var publicKey interface{}
		if err := keys.Raw(&publicKey); err != nil {
			return nil, err
		}

		return publicKey, nil
	})

	return token, err
}