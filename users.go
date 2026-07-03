package main

import (
	"fmt"
)

// 500 hardcoded users, generated deterministically at startup — no database.
// user001/Pass001! … user500/Pass500!
const userCount = 500

var users = buildUsers()

func buildUsers() map[string]string {
	m := make(map[string]string, userCount)
	for i := 1; i <= userCount; i++ {
		m[fmt.Sprintf("user%03d", i)] = fmt.Sprintf("Pass%03d!", i)
	}
	return m
}

func checkCredentials(username, password string) bool {
	p, ok := users[username]
	return ok && p == password
}
