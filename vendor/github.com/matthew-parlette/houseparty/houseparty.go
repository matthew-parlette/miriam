package houseparty

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/RocketChat/Rocket.Chat.Go.SDK/models"
	chat "github.com/RocketChat/Rocket.Chat.Go.SDK/realtime"
	"github.com/adlio/trello"
	jira "github.com/andygrunwald/go-jira"
	"github.com/heptiolabs/healthcheck"
	"github.com/sachaos/todoist/lib"
)

var (
	ConfigPath    string
	SecretsPath   string
	JiraClient    *jira.Client
	TodoistClient *todoist.Client
	ChatClient    *chat.Client
	TrelloClient  *trello.Client
)

func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

func Config(item string) string {
	contents, err := ioutil.ReadFile(path.Join(ConfigPath, item))
	if err != nil {
		log.Fatal(err)
		return ""
	}
	result := strings.TrimSpace(string(contents))
	return result
}

func Secret(item string) string {
	contents, err := ioutil.ReadFile(path.Join(SecretsPath, item))
	if err != nil {
		log.Fatal(err)
		return ""
	}
	result := strings.TrimSpace(string(contents))
	return result
}

func InitJiraClient() error {
	tp := jira.BasicAuthTransport{
		Username: Config("jira-username"),
		Password: Secret("jira-password"),
	}
	client, err := jira.NewClient(tp.Client(), Config("jira-url"))
	JiraClient = client
	if err != nil {
		return err
	}
	return nil
}

func InitTodoistClient() error {
	config := &todoist.Config{
		AccessToken: Secret("todoist-token"),
		DebugMode:   false,
		// Color:       false,
	}
	client := todoist.NewClient(config)
	var store todoist.Store
	client.Store = &store
	TodoistClient = client
	return nil
}

func InitRocketChatClient() error {
	rocketchatUrlString := Config("rocketchat-url")
	rocketchatUrl, err := url.Parse(rocketchatUrlString)
	if err != nil {
		return err
	}
	client, err := chat.NewClient(rocketchatUrl, false)
	if err != nil {
		return err
	}
	_, err = client.Login(&models.UserCredentials{
		Email:    Config("rocketchat-email"),
		Password: Secret("rocketchat-password")})
	if err != nil {
		return err
	}
	ChatClient = client
	return nil
}

func GetChatChannel(channel string) models.Channel {
	channel_id, _ := ChatClient.GetChannelId(channel)
	return models.Channel{ID: channel_id}
}

func SendChatMessage(channel string, message string) error {
	ch := GetChatChannel("house-party")
	ChatClient.SendMessage(&ch, message)
	return nil
}

func GetNonBotUsers() []string {
	// rawResponse, err := ChatClient.ddp.Call("getUserRoles")
	// if err != nil {
	// 	return []string
	// }
	// document, _ := gabs.Consume(rawResponse)
	// roles, err := document.Children()
	// result = []string
	// for _, role := range roles {
	// 	result = append(result, role["username"])
	// }
	return []string{"matt"}
}

func IsNonBotUser(user string, nonBotUsers []string) bool {
	for _, u := range nonBotUsers {
		if u == user {
			return true
		}
	}
	return false
}

func StartChatListener() error {
	// if ChatClient == nil {
	// 	return errors.New("houseparty.ChatClient is nil")
	// }
	channel := GetChatChannel("house-party")
	messageChannel := make(chan models.Message, 1)
	if err := ChatClient.SubscribeToMessageStream(&channel, messageChannel); err != nil {
		return err
	}
	shutdown := make(chan struct{})
	nonBotUsers := GetNonBotUsers()
	fmt.Println("Only listening for messages from", nonBotUsers)
	go func() {
		for {
			select {
			case msg := <-messageChannel:
				// fmt.Println("I saw a message with text:", msg)
				if IsNonBotUser(msg.User.UserName, nonBotUsers) {
					if strings.Contains(msg.Text, "status") || strings.Contains(msg.Text, "check in") {
						SendChatMessage("house-party", "I'm online")
					}
					if strings.Contains(msg.Text, "help") || strings.Contains(msg.Text, "commands") {
						response := "Here are commands I can respond to:"
						// response = fmt.Sprintf("%v\n```", response)
						response = fmt.Sprintf("%v\n> *status*: See if I am online", response)
						response = fmt.Sprintf("%v\n> *check in*: See if I am online", response)
						response = fmt.Sprintf("%v\n> *help*: Get a list of commands", response)
						response = fmt.Sprintf("%v\n> *commands*: Get a list of commands", response)
						// response = fmt.Sprintf("%v\n```", response)
						SendChatMessage("house-party", response)
					}
				}
			case <-shutdown:
				return
			}
		}
	}()
	return nil
}

func InitTrelloClient() error {
	client := trello.NewClient(Secret("trello-key"), Secret("trello-token"))
	fmt.Println(client)
	TrelloClient = client
	return nil
}

func StartHealthCheck() error {
	health := healthcheck.NewHandler()
	// Our app is not happy if we've got more than 100 goroutines running.
	health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
	// Our app is not ready if we can't resolve our upstream dependencies in DNS.
	health.AddReadinessCheck("todoist-dns", healthcheck.DNSResolveCheck("www.todoist.com", 5000*time.Millisecond))
	// chatUrl, err := url.Parse(Config("rocketchat-url"))
	// if err != nil {
	// 	return err
	// }
	// health.AddReadinessCheck("chat-dns", healthcheck.DNSResolveCheck(chatUrl.Host, 5000*time.Millisecond))
	jiraUrl, err := url.Parse(Config("jira-url"))
	if err != nil {
		return err
	}
	health.AddReadinessCheck("jira-dns", healthcheck.DNSResolveCheck(jiraUrl.Host, 5000*time.Millisecond))
	go http.ListenAndServe("0.0.0.0:8086", health)
	return nil
}

func init() {
	ConfigPath = GetEnv("CONFIG_PATH", "config")
	SecretsPath = GetEnv("SECRETS_PATH", "secrets")
	fmt.Println("Initializing JIRA...")
	if err := InitJiraClient(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Initializing todoist...")
	if err := InitTodoistClient(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Initializing rocketchat...")
	if err := InitRocketChatClient(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Initializing trello...")
	if err := InitTrelloClient(); err != nil {
		log.Fatal(err)
	}
}
