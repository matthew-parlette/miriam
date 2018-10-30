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
	"github.com/sachaos/todoist/lib"
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

func AddChecklist(card *trello.Card, name string) error {
	path := fmt.Sprintf("cards/%s/checklists", card.ID)
	err := houseparty.TrelloClient.Post(path, trello.Arguments{"name": name}, &card.IDCheckLists)
	if err != nil {
		err = errors.Wrapf(err, "Error creating checklist on card %s", card.ID)
	}
	return err
}

// Return the checked and unchecked items for a checklist
// Create the checklist if necessary
func getChecklistItems(card *trello.Card, name string) ([]trello.CheckItem, []trello.CheckItem) {
	var checked []trello.CheckItem
	var unchecked []trello.CheckItem

	// Does it already exist?
	for _, existingChecklist := range card.Checklists {
		if existingChecklist.Name == name {
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
	}

	// It doesn't exist, create it
	AddChecklist(card, name)
	return checked, unchecked
}

func hasLabel(card *trello.Card, name string) bool {
	for _, label := range card.Labels {
		if label.Name == name {
			return true
		}
	}
	fmt.Println("Couldn't find", name, "label on card", card.ID)
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
func findExistingTodoistTask(content string, strict bool) []todoist.Item {
	var existing []todoist.Item
	for _, item := range houseparty.TodoistClient.Store.Items {
		if strict {
			if item.Content == content {
				existing = append(existing, item)
			}
		} else {
			if strings.Contains(item.Content, content) {
				existing = append(existing, item)
			}
		}
	}
	return existing
}

func run() {
	syncTodoist()
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
		todoistProjectID := getTodoistWorkingProjectID()
		if card.List.Name == "In Progress" {
			// successChecked, successUnchecked := getChecklistItems(card, "Success Criteria")
			tasksChecked, tasksUnchecked := getChecklistItems(card, "Tasks")
			// backlogChecked, backlogUnchecked := getChecklistItems(card, "Backlog")
			// TODO: Move completed backlog checklist items to tasks
			// Sync task checklist items with todoist. On conflict, todoist wins
			for _, item := range tasksChecked {
				tasks := findExistingTodoistTask(item.Name, false)
				if len(tasks) > 0 {
					// Found at least one todoist task
					for _, task := range tasks {
						fmt.Printf("Found a todoist task (%v) that matches complete checklist item (%v)\n", task.Content, item.Name)
						if task.Checked == 0 {
							// Checklist item is complete, Todoist task is not
							fmt.Println("Todoist task is incomplete, but checklist item is complete, marking Todoist task as complete...")
						}
					}
				}
			}
			for _, item := range tasksUnchecked {
				tasks := findExistingTodoistTask(item.Name, false)
				if len(tasks) > 0 {
					// Found at least one matching todoist task
					for _, task := range tasks {
						fmt.Printf("Found a todoist task (%v) that matches incomplete checklist item (%v)\n", task.Content, item.Name)
						if task.Checked == 0 {
							// Checklist item is incomplete, Todoist task is as well
							fmt.Println("Todoist task and Trello checklist item are both incomplete, moving on...")
						} else {
							// Checklist item is incomplete, Todoist task is complete
							fmt.Println("Todoist task is complete, Trello checklist item is incomplete, marking Trello checklist item as complete...")
						}
					}
				} else {
					fmt.Printf("Todoist task is missing, creating one from Trello checklist item (%v)...\n", item.Name)
					t := todoist.Item{
						Priority:   4,
						DateString: "tod",
					}
					t.Content = fmt.Sprintf("[%v](%v)", item.Name, card.Url)
					t.ProjectID = todoistProjectID
					if err := houseparty.TodoistClient.AddItem(context.Background(), t); err != nil {
						err = errors.Wrapf(err, "Error creating todoist task: %s", t.Content)
						log.Println(err)
						continue
					}
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
