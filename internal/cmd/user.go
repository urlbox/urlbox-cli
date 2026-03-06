package cmd

import (
	"github.com/urlbox/cli/internal/api"
)

func fetchCurrentUser(client *api.Client) (map[string]interface{}, error) {
	var response map[string]interface{}
	if err := client.GetJSON("/v1/users/me", &response); err != nil {
		return nil, err
	}

	user, _ := response["user"].(map[string]interface{})
	return user, nil
}

func findProjectByID(user map[string]interface{}, projectID string) map[string]interface{} {
	projects, _ := user["projects"].([]interface{})
	for _, project := range projects {
		projectMap, _ := project.(map[string]interface{})
		if projectMap == nil {
			continue
		}
		if id, _ := projectMap["_id"].(string); id == projectID {
			return projectMap
		}
	}
	return nil
}
