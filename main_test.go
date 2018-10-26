package main

import (
	"testing"

	"github.com/matthew-parlette/houseparty"
)

func TestRun(t *testing.T) {
	// t.Skip("Skipping run test")
	run()
}

func TestChatListener(t *testing.T) {
	t.Skip("Skipping chat listener test")
	houseparty.StartChatListener()
}

func TestTrelloObject(t *testing.T) {
	t.Skip("Skipping Todoist object test")
	// if syncTodoist() {
	// 	spew.Dump(houseparty.TodoistClient.Store.Items[0])
	// }
}
