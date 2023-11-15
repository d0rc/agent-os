package tools

import (
	"math/rand"
	"strings"
)

func GetSystemName(name string) string {
	resultingName := "agent-" + name
	resultingName = strings.Replace(resultingName, " ", "-", -1)
	resultingName = strings.ToLower(resultingName)
	randomSuffix := randomString(4)
	resultingName += "-" + randomSuffix

	return resultingName
}

var digits = []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}

func randomString(n int) string {
	// generate n random digits
	// convert them to string
	var result string
	for i := 0; i < n; i++ {
		result += digits[rand.Intn(len(digits))]
	}

	return result
}
