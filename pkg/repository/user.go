package repository

import (
	"context"

	"github.com/machinebox/graphql"
)

type User struct {
	ID      int    `json:"id"`
	Itsc    string `json:"itsc"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"isAdmin"`
	Courses []struct {
		CourseID int `json:"course_id"`
	} `json:"courses"`
}

func (r *repository) GetUser(ctx context.Context, itsc, name string) (*User, error) {
	req := graphql.NewRequest(GetUserByITSC)
	req.Var("itsc", itsc)

	var resp struct {
		Users []User `json:"users"`
	}

	if err := r.client.Run(ctx, req, &resp); err != nil {
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

	if err := r.client.Run(ctx, req, &createResp); err != nil {
		return nil, err
	}

	return &User{ID: createResp.InsertUsersOne.ID, Name: name, Itsc: itsc}, nil
}

func (r *repository) GetStudentUserIds(ctx context.Context, itscIDs []string) ([]int, error) {
	req := graphql.NewRequest(getStudentUserIds)
	req.Var("itscIds", itscIDs)

	var resp struct {
		Users []struct {
			ID   int    `json:"id"`
			ITSC string `json:"itsc"`
		} `json:"users"`
	}

	if err := r.client.Run(ctx, req, &resp); err != nil {
		return nil, err
	}

	var userIDs []int
	var existingITSCs []string
	for _, user := range resp.Users {
		userIDs = append(userIDs, user.ID)
		existingITSCs = append(existingITSCs, user.ITSC)
	}

	var newITSCs []string
	for _, itscID := range itscIDs {
		found := false
		for _, existingITSC := range existingITSCs {
			if itscID == existingITSC {
				found = true
				break
			}
		}
		if !found {
			newITSCs = append(newITSCs, itscID)
		}
	}

	if len(newITSCs) > 0 {
		var users []map[string]interface{}
		for _, itsc := range newITSCs {
			users = append(users, map[string]interface{}{"itsc": itsc})
		}

		req := graphql.NewRequest(addUsers)
		req.Var("users", users)

		var addResp struct {
			BatchCreateUser struct {
				Returning []struct {
					ID int `json:"id"`
				} `json:"returning"`
			} `json:"batchCreateUser"`
		}

		if err := r.client.Run(ctx, req, &addResp); err != nil {
			return nil, err
		}

		for _, user := range addResp.BatchCreateUser.Returning {
			userIDs = append(userIDs, user.ID)
		}
	}

	return userIDs, nil
}
