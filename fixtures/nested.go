package fixtures

import "strings"

type User struct {
	Name   string
	Active bool
}

func normalizeActiveNames(users []User) []string {
	result := make([]string, 0, len(users))
	for _, user := range users {
		if !user.Active {
			continue
		}
		result = append(result, strings.ToLower(strings.TrimSpace(user.Name)))
	}
	return result
}
