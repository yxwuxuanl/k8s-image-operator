package controller

import "os"

func intersection(a, b []string) []string {
	if len(a) == 0 {
		return b
	}

	m := make(map[string]bool)
	for _, num := range a {
		m[num] = true
	}

	var intersect []string
	for _, num := range b {
		if _, found := m[num]; found {
			intersect = append(intersect, num)
			delete(m, num)
		}
	}
	return intersect
}

func getEnvOrDie(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic("missing required environment variable " + name)
	}
	return value
}
