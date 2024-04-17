package controller

import "os"

func getEnvOrDie(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic("missing required environment variable " + name)
	}
	return value
}
