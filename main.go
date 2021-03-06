package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/adlio/trello"
	"github.com/matthew-parlette/houseparty"
	"github.com/pkg/errors"
	wunderlist "github.com/robdimsdale/wl"
)

// Trello

func getCards(board *trello.Board) []*trello.Card {
	var cards []*trello.Card
	lists, err := board.GetLists(trello.Defaults())
	if err != nil {
		// Handle error
	}
	for _, list := range lists {
		if list.Name != "Ideas" && list.Name != "Needs research" {
			listCards, err := list.GetCards(trello.Defaults())
			if err != nil {
				// Handle error
			}
			cards = append(cards, listCards...)
		}
	}
	// excludes := houseparty.Config("backlog-excludes")
	// cards := []trello.Card
	return cards
}

func getListByName(board *trello.Board, name string) *trello.List {
	lists, _ := board.GetLists(trello.Arguments{})
	for _, list := range lists {
		if list.Name == name {
			return list
		}
	}
	return nil
}

func AddChecklist(card *trello.Card, name string) error {
	path := fmt.Sprintf("cards/%s/checklists", card.ID)
	err := houseparty.TrelloClient.Post(path, trello.Arguments{"name": name}, &card.IDCheckLists)
	if err != nil {
		err = errors.Wrapf(err, "Error creating checklist on card %s", card.ID)
	}
	return err
}

func MarkChecklistItem(card *trello.Card, item trello.CheckItem, state string) error {
	path := fmt.Sprintf("cards/%s/checkItem/%s", card.ID, item.ID)
	err := houseparty.TrelloClient.Put(path, trello.Arguments{"state": state}, &card.IDCheckLists)
	if err != nil {
		err = fmt.Errorf("Error marking checklist item '%s' as %s: %s", item.Name, state, err)
	}
	return err
}

func getChecklist(card *trello.Card, name string) *trello.Checklist {
	for _, existingChecklist := range card.Checklists {
		if existingChecklist.Name == name {
			return existingChecklist
		}
	}
	return nil
}

// Return the checked and unchecked items for a checklist
// Create the checklist if necessary
func getChecklistItems(card *trello.Card, name string) ([]trello.CheckItem, []trello.CheckItem) {
	var checked []trello.CheckItem
	var unchecked []trello.CheckItem

	// Does it already exist?
	existingChecklist := getChecklist(card, name)
	if existingChecklist != nil {
		for _, item := range existingChecklist.CheckItems {
			if item.State == "complete" {
				checked = append(checked, item)
			}
			if item.State == "incomplete" {
				unchecked = append(unchecked, item)
			}
		}
		return checked, unchecked
	}

	// It doesn't exist, create it
	AddChecklist(card, name)
	return checked, unchecked
}

func moveItemToChecklist(item trello.CheckItem, card *trello.Card, name string) error {
	newChecklist := getChecklist(card, name)
	if newChecklist == nil {
		return fmt.Errorf("Could not find checklist '%v'", name)
	}
	path := fmt.Sprintf("cards/%s/checkItem/%s", card.ID, item.ID)
	err := houseparty.TrelloClient.Put(path, trello.Arguments{"idChecklist": newChecklist.ID}, &card.IDCheckLists)
	if err != nil {
		err = errors.Wrapf(err, "Error moving checklist item '%s' to %s", item.Name, name)
	}
	return err
}

func hasLabel(card *trello.Card, name string) bool {
	for _, label := range card.Labels {
		if label.Name == name {
			return true
		}
	}
	return false
}

func addLabel(card *trello.Card, name string) {
	labels, err := card.Board.GetLabels(trello.Arguments{})
	if err != nil {
		log.Fatal(err)
		return
	}
	for _, label := range labels {
		if label.Name == name {
			card.AddIDLabel(label.ID)
		}
	}
}

func removeLabel(card *trello.Card, name string) {
	for _, label := range card.Labels {
		if label.Name == name {
			card.RemoveIDLabel(label.ID, label)
		}
	}
}

// Todoist

func syncTodoist() bool {
	if err := houseparty.TodoistClient.Sync(context.Background()); err != nil {
		log.Fatal(err)
		return false
	}
	return true
}

func getTodoistWorkingProjectID() int {
	project := 0
	search := houseparty.Config("todoist-project")
	for _, p := range houseparty.TodoistClient.Store.Projects {
		if p.Name == search {
			project = p.GetID()
		}
	}

	if project == 0 {
		log.Fatal("Could not find project with name ", houseparty.Config("todoist-project"))
	}

	return project
}

// Find an existing todoist task with the given content
// To match the entire content, strict == true
func findExistingTasks(tasks []wunderlist.Task, content string, strict bool) []wunderlist.Task {
	var existing []wunderlist.Task
	for _, item := range tasks {
		if strict {
			if item.Title == content {
				existing = append(existing, item)
			}
		} else {
			if strings.Contains(item.Title, content) {
				existing = append(existing, item)
			}
		}
	}
	return existing
}

func run() {
	fmt.Println("Starting run at", time.Now().Format("2006-01-02T15:04:05-0700"))
	inbox, _ := houseparty.WunderlistClient.Inbox()
	wunderlistUser, _ := houseparty.WunderlistClient.User()
	inboxTasks, err := houseparty.WunderlistClient.TasksForListID(inbox.ID)
	if err != nil {
		log.Printf("Error loading open tasks: %v", err)
		log.Printf("Skipping this run, will try again in %v seconds...", houseparty.Config("interval"))
		return
	}
	inboxCompleted, _ := houseparty.WunderlistClient.CompletedTasksForListID(inbox.ID, true)
	inboxTasks = append(inboxTasks, inboxCompleted...)
	fmt.Printf("Found %v tasks (%v completed)\n", len(inboxTasks), len(inboxCompleted))
	backlogBoard, err := houseparty.TrelloClient.GetBoard(houseparty.Config("trello-backlog"), trello.Defaults())
	if err != nil {
		log.Fatal(err)
		return
	}
	goalsBoard, err := houseparty.TrelloClient.GetBoard(houseparty.Config("trello-goals"), trello.Defaults())
	if err != nil {
		log.Fatal(err)
		return
	}
	// If there are no cards in In Progress, but there are cards in To Do, move one to In Progress
	inProgressList := getListByName(goalsBoard, "In Progress")
	if inProgressList != nil {
		cards, _ := inProgressList.GetCards(trello.Arguments{})
		if len(cards) == 0 {
			toDoList := getListByName(goalsBoard, "To Do")
			toDoCards, _ := toDoList.GetCards(trello.Arguments{})
			if len(toDoCards) > 0 {
				fmt.Printf("In Progress list is empty, moving To Do card %v to In Progress...", toDoCards[0].Name)
				toDoCards[0].MoveToList(inProgressList.ID, trello.Arguments{})
			} else {
				fmt.Printf("No cards in 'In Progress' or 'To Do', creating a task to plan one...")
				houseparty.WunderlistClient.CreateTask(fmt.Sprintf("Start working on a new goal (%v)", goalsBoard.ShortUrl), inbox.ID, wunderlistUser.ID, false, "", 0, time.Now().Local(), false)
			}
		}
	}

	for _, card := range getCards(backlogBoard) {
		// Need to get full card details to get checklists
		card, err = houseparty.TrelloClient.GetCard(card.ID, trello.Arguments{
			"checklists": "all",
		})
		if err != nil {
			log.Fatal(err)
			continue
		}
		successChecked, successUnchecked := getChecklistItems(card, "Success Criteria")
		tasksChecked, tasksUnchecked := getChecklistItems(card, "Tasks")
		backlogChecked, backlogUnchecked := getChecklistItems(card, "Backlog")

		// If checklists are empty, add labels
		// First, we need to load the Board into the Card object
		card.Board, err = houseparty.TrelloClient.GetBoard(card.IDBoard, trello.Arguments{})
		if err != nil {
			log.Fatal(err)
			continue
		}
		if len(successChecked)+len(successUnchecked) == 0 {
			addLabel(card, "Needs success criteria")
		} else {
			removeLabel(card, "Needs success criteria")
		}
		if len(tasksChecked)+len(tasksUnchecked)+len(backlogChecked)+len(backlogUnchecked) == 0 {
			addLabel(card, "Needs tasks")
		} else {
			removeLabel(card, "Needs tasks")
		}

		// If card is marked as planned, move it to the goals board
		if hasLabel(card, "Planned") {
			// Remove Planned label before moving
			removeLabel(card, "Planned")
			// Then move the card
			fmt.Println("Moving card", card.ID, "to board", goalsBoard.ID)
			if err := card.Update(trello.Arguments{"idBoard": goalsBoard.ID}); err != nil {
				err = errors.Wrapf(err, "Error moving card %s to board %s", card.ID, goalsBoard.ID)
				log.Println(err)
				continue
			}
		}
	}
	for _, card := range getCards(goalsBoard) {
		// Need to get full card details to get checklists
		card, err = houseparty.TrelloClient.GetCard(card.ID, trello.Arguments{
			"checklists": "all",
			"list":       "true",
		})
		if err != nil {
			log.Fatal(err)
			continue
		}
		if card.List.Name == "In Progress" {
			// successChecked, successUnchecked := getChecklistItems(card, "Success Criteria")
			tasksChecked, tasksUnchecked := getChecklistItems(card, "Tasks")
			backlogChecked, backlogUnchecked := getChecklistItems(card, "Backlog")
			// TODO: Remove tasks for uncheck backlog items
			if len(backlogUnchecked) > 0 {
				for _, item := range backlogUnchecked {
					tasks := findExistingTasks(inboxTasks, item.Name, false)
					if len(tasks) > 0 {
						for _, task := range tasks {
							fmt.Printf("    Found task for unchecked backlog item, deleting task...\n")
							houseparty.WunderlistClient.DeleteTask(task)
						}
					}
				}
			}
			// TODO: Move completed backlog checklist items to tasks
			if len(backlogChecked) > 0 {
			}
			// TODO: Move incomplete backlog item to tasks
			if len(tasksUnchecked) == 0 && len(backlogUnchecked) > 0 {
				nextItem := backlogUnchecked[0]
				fmt.Printf("All items in %v for card '%v' are completed, moving %v item (%v) to %v...\n", "Tasks", card.Name, "Backlog", nextItem.Name, "Tasks")
				moveItemToChecklist(nextItem, card, "Tasks")
				// Reload the tasks since we just moved an item to Tasks
				tasksChecked, tasksUnchecked = getChecklistItems(card, "Tasks")
			}
			// Sync task checklist items with wunderlist. On conflict, wunderlist wins
			for _, item := range tasksChecked {
				fmt.Printf("Processing checked checklist item (%v)...\n", item.Name)
				tasks := findExistingTasks(inboxTasks, item.Name, false)
				if len(tasks) > 0 {
					// Found at least one task
					for _, task := range tasks {
						fmt.Printf("    Found a matching task (%v)\n", task.Title)
						if task.Completed == false {
							// Checklist item is complete, task is not
							fmt.Println("    Task is incomplete, but checklist item is complete, marking task as complete...")
							task.Completed = true
							houseparty.WunderlistClient.UpdateTask(task)
						} else {
							fmt.Println("    Task and checklist item are both complete, moving on...")
						}
					}
				} else {
					fmt.Println("    Task is missing, moving on...")
				}
			}
			for _, item := range tasksUnchecked {
				fmt.Printf("Processing unchecked checklist item (%v)...\n", item.Name)
				tasks := findExistingTasks(inboxTasks, item.Name, false)
				if len(tasks) > 0 {
					// Found at least one matching todoist task
					for _, task := range tasks {
						fmt.Printf("    Found a matching task (%v)\n", task.Title)
						if task.Completed == false {
							// Checklist item is incomplete, task is as well
							fmt.Println("    Task and checklist item are both incomplete, moving on...")
						} else {
							// Checklist item is incomplete, task is complete
							fmt.Println("    Task is complete, checklist item is incomplete, marking checklist item as complete...")
							_ = MarkChecklistItem(card, item, "complete")
						}
					}
				} else {
					fmt.Printf("    Task is missing, creating one from checklist item (%v)...\n", item.Name)
					houseparty.WunderlistClient.CreateTask(fmt.Sprintf("%v (%v)", item.Name, card.ShortUrl), inbox.ID, wunderlistUser.ID, false, "", 0, time.Now().Local(), false)
				}
			}
		}
	}
	fmt.Printf("Waiting %v seconds to run again...\n", houseparty.Config("interval"))
}

func init() {
	houseparty.ConfigPath = houseparty.GetEnv("CONFIG_PATH", "config")
	houseparty.SecretsPath = houseparty.GetEnv("SECRETS_PATH", "secrets")
}

func main() {
	fmt.Println("Initializing...")
	houseparty.StartHealthCheck()
	interval, err := strconv.Atoi(houseparty.Config("interval"))
	if err != nil {
		log.Fatal(err)
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	shutdown := make(chan struct{})

	if houseparty.ChatClient != nil {
		houseparty.StartChatListener()
	}

	fmt.Println("Initialization complete")

	// First run before waiting for ticker
	run()

	go func() {
		for {
			select {
			case <-ticker.C:
				run()
			case <-shutdown:
				ticker.Stop()
				return
			}
		}
	}()

	// block forever
	<-shutdown
}
