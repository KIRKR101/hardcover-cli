package cmd

import (
	"context"

	"github.com/KIRKR101/hardcover-cli/internal/api"
)

// getMe fetches the authenticated user. Used by every command that
// needs the user_id. The token is loaded from config and a Client is
// constructed each call — cheap, and avoids passing it everywhere.
func getMe(ctx context.Context, c *api.Client) (api.User, error) {
	var resp struct {
		Me []api.User `json:"me"`
	}
	if err := c.GQL(ctx, api.QueryMe, nil, &resp); err != nil {
		return api.User{}, err
	}
	if len(resp.Me) == 0 {
		return api.User{}, nil
	}
	return resp.Me[0], nil
}
