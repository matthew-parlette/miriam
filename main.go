package main

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/adlio/trello"
	"github.com/matthew-parlette/houseparty"
	"github.com/pkg/errors"
)

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
func getChecklistStats(card *trello.Card, name string) (int, int) {
	checked := 0
	unchecked := 0

	// Does it already exist?
	for _, existingChecklist := range card.Checklists {
		if existingChecklist.Name == name {
			for _, item := range existingChecklist.CheckItems {
				if item.State == "complete" {
					checked = checked + 1
				}
				if item.State == "incomplete" {
					unchecked = unchecked + 1
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
	// labels, err := card.Board.GetLabels(trello.Arguments{})
	// if err != nil {
	// 	log.Fatal(err)
	// 	return false
	// }
	for _, label := range card.Labels {
		if label.Name == name {
			fmt.Printf("Found %v\n", name)
			return true
		}
	}
	fmt.Printf("Didn't fint %v\n", name)
	return false
}

func addLabel(card *trello.Card, name string) {
	fmt.Println(card.Board)
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

func run() {
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
		successChecked, successUnchecked := getChecklistStats(card, "Success Criteria")
		tasksChecked, tasksUnchecked := getChecklistStats(card, "Tasks")
		backlogChecked, backlogUnchecked := getChecklistStats(card, "Backlog")

		// If checklists are empty, add labels
		// First, we need to load the Board into the Card object
		card.Board, err = houseparty.TrelloClient.GetBoard(card.IDBoard, trello.Arguments{})
		if err != nil {
			log.Fatal(err)
			continue
		}
		if successChecked+successUnchecked == 0 {
			addLabel(card, "Needs success criteria")
		} else {
			removeLabel(card, "Needs success criteria")
		}
		if tasksChecked+tasksUnchecked+backlogChecked+backlogUnchecked == 0 {
			addLabel(card, "Needs tasks")
		} else {
			removeLabel(card, "Needs tasks")
		}

		// If card is marked as planned, move it to the goals board
		if hasLabel(card, "Planned") {
			// Remove Planned label before moving
			removeLabel(card, "Planned")
			// Then move the card
			if err := card.Update(trello.Arguments{"idBoard": goalsBoard.ID}); err != nil {
				err = errors.Wrapf(err, "Error moving card %s to board %s", card.ID, goalsBoard.ID)
				log.Println(err)
				continue
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
