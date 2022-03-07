package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/bwmarrin/discordgo"
	v3 "github.com/google/go-github/v43/github"
	"github.com/mehanizm/airtable"
	ghwebhooks "gopkg.in/go-playground/webhooks.v5/github"
)

var (
	dg        *discordgo.Session
	shibeRole *discordgo.Role
	table     *airtable.Table

	c = getCfg()

	msgIDToUser = make(map[string]*discordgo.User, 100)
	ghUser      = make(map[string]*v3.User, 100)

	//appID = int64(142994) // your app id goes here
	//user  = "quackduck"
	//
	//privkey = ""

	installationID int64
	itr            *ghinstallation.Transport
	ghBotSlug      = ""
)

type config struct {
	Token             string
	TargetRoleName    string
	ServerID          string
	ChannelID         string
	AirtableKey       string
	AirtableBaseID    string
	AirtableTableName string
	GHPrivKey         string
	GHUser            string
	GHAppID           int64
}

func getCfg() *config {
	b, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(0)
	}
	cfg := new(config)
	err = json.Unmarshal(b, cfg)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(0)
	}
	return cfg
}

func main() {
	var err error
	rand.Seed(time.Now().UnixNano())

	dg, err = discordgo.New("Bot " + c.Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}
	dg.AddHandler(memberAdd)
	dg.AddHandler(msgReact)
	dg.Identify.Intents = discordgo.IntentsGuildMessageReactions | discordgo.IntentsGuildMembers

	err = setup()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	err = dg.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	atr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, c.GHAppID, []byte(c.GHPrivKey))
	if err != nil {
		fmt.Println("error creating GitHub app client")
		return
	}

	//
	//installation, _, err := v3.NewClient(&http.Client{Transport: atr}).Apps.FindOrganizationInstallation(context.TODO(), orgID)
	//if err != nil {
	//	log.Fatalf("error finding organization installation: %v", err)
	//}

	installation, _, err := v3.NewClient(&http.Client{Transport: atr}).Apps.FindUserInstallation(context.TODO(), c.GHUser)
	if err != nil {
		fmt.Printf("error finding installation: %v", err)
		return
	}

	ghBotSlug = *installation.AppSlug
	installationID = installation.GetID()
	itr = ghinstallation.NewFromAppsTransport(atr, installationID)

	fmt.Printf("Successfully initialized GitHub app client, installation-id:%d expected-events:%v\n", installationID, installation.Events)
	http.HandleFunc("/github", Handle)
	err = http.ListenAndServe("0.0.0.0:3000", nil)
	if err != nil && err != http.ErrServerClosed {
		fmt.Println(err)
		return
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	dg.Close()
}

// finds the shibeRole role
func setup() error {
	roles, err := dg.GuildRoles(c.ServerID)
	if err != nil {
		return err
	}
	fmt.Println("Looking through server roles...")
	for i := range roles {
		if roles[i].Name == c.TargetRoleName {
			shibeRole = roles[i]
		}
	}
	if shibeRole == nil {
		return errors.New("no '" + c.TargetRoleName + "' role detected, you should add one")
	}
	fmt.Println("Found the role!")

	client := airtable.NewClient(c.AirtableKey)
	//client.SetBaseURL("https://api.airtable.com/v0/")
	table = client.GetTable(c.AirtableBaseID, c.AirtableTableName)
	return nil
}

func memberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	fmt.Println(m.User.Username)
	msg, err := s.ChannelMessageSend(c.ChannelID,
		pickRandom(
			[]string{
				m.Member.Mention() + ", you have just joined my list of subjects! You should react with 👍 to accept the legal stuff, \nMuch thanks",
				"OMG welcome to my dogedom " + m.Member.Mention() + "!! React with 👍 and I'll have my secretary note that you accepted the CLA.\nI'll also give you your Shibe role, so wow!",
				"Yay, one more doge to rule over! Accept the Dogecoin CLA by reacting with 👍 and you'll get the rare Shibe role, " + m.Member.Mention() + ". Very excitement!!",
				"Wow, so many people like Dogecoin. You should accept the CLA as fast as you can, just react with 👍, so I can give you access to all the channels, " + m.Member.Mention(),
				"QUICKLYY!! React with 👍 to accept the CLA and you'll be on your doge way " + m.Member.Mention(),
				"Another person who Does Only Good Everyday? Great, you should accept the CLA by reacting with 👍 and you'll be an official shibe " + m.Member.Mention() + "!!!",
			},
		),
	)
	if err != nil {
		fmt.Println("error:", err)
	}
	msgIDToUser[msg.ID] = m.User
}

func pickRandom(a []string) string {
	return a[rand.Intn(len(a))]
}

func msgReact(s *discordgo.Session, m *discordgo.MessageReactionAdd) {
	// could be replaced with an airtable-exists check but this could be faster and cause less airtable requests
	// has the side effect of not persisting the list of people left to react to the message
	if user, ok := msgIDToUser[m.MessageID]; ok {
		delete(msgIDToUser, m.MessageID)
		if m.UserID == user.ID && (m.Emoji.Name == "👍") {
			addApprovedDiscordUser(user, discordMessageToLink(m))
			_, err := s.ChannelMessageSend(c.ChannelID, "Wow, thanks for your acceptance, "+user.Mention()+". I'm adding the Shibe role yahoo!")
			if err != nil {
				fmt.Println("Couldn't send a message. error: ", err)
			}
			err = s.GuildMemberRoleAdd(c.ServerID, user.ID, shibeRole.ID)
			if err != nil {
				_, err = s.ChannelMessageSend(c.ChannelID, "OH NOOOOO I got an error while trying to make you a shibe: "+err.Error()+"\n Please send much stern messages to Ishan#9106")
				if err != nil {
					fmt.Println("Couldn't send a message. error: ", err)
				}
			}
		}
	}
}

func discordMessageToLink(m *discordgo.MessageReactionAdd) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", m.GuildID, m.ChannelID, m.MessageID)
}

func addApprovedDiscordUser(user *discordgo.User, messageLink string) {
	addToAirtable(user.String(), "Discord: Agreed to the CLA by reacting with 👍 here: "+messageLink)
}

func addToAirtable(name string, notes string) {
	exists, err := doesRecordExist(name)
	if err != nil {
		fmt.Println("error checking if record exists already:", err)
	}
	if exists {
		return
	}
	recordsToSend := &airtable.Records{
		Records: []*airtable.Record{
			{
				Fields: map[string]interface{}{
					"Name":  name,
					"Notes": notes,
				},
			},
		},
	}
	records, err := table.AddRecords(recordsToSend)
	if err != nil {
		fmt.Println("error: ", err)
		return
	}
	j, err := json.MarshalIndent(records, "", "   ")
	if err != nil {
		fmt.Println("error: ", err)
	}
	fmt.Println("Sent these records to Airtable:", string(j))
}

func doesRecordExist(name string) (bool, error) {
	fmt.Println("Checking if record exists for:", name)
	checkExistsURLValues := url.Values{}
	//checkExistsURLValues.Add("fields", "Name") // not necessary + airtable really doesn't like me picking only a single field for some reason
	// This change made it work and so I said this: https://github.com/quackduck/test-gh-go-bot/pull/17#issuecomment-1061226233
	checkExistsURLValues.Add("filterByFormula", fmt.Sprintf("{Name} = '%s'", name))
	records, err := table.GetRecordsWithParams(checkExistsURLValues)
	if err != nil {
		return false, err
	}
	fmt.Println("records found for", name, ToJSON(records))
	if len(records.Records) > 0 { // there are duplicates
		return true, nil
	}
	return false, nil
}

func Handle(response http.ResponseWriter, request *http.Request) {
	hook, err := ghwebhooks.New()
	if err != nil {
		return
	}

	payload, err := hook.Parse(request, []ghwebhooks.Event{ghwebhooks.ReleaseEvent, ghwebhooks.PullRequestEvent, ghwebhooks.IssueCommentEvent, ghwebhooks.PullRequestReviewCommentEvent}...)
	if err != nil {
		if err == ghwebhooks.ErrEventNotFound {
			fmt.Printf("received unregistered GitHub event: %v\n", err)
			response.WriteHeader(http.StatusOK)
		} else {
			fmt.Printf("received malformed GitHub event: %v\n", err)
			response.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	switch payload := payload.(type) {
	case ghwebhooks.PullRequestPayload:
		fmt.Println("received pull request event, payload action & URL:", payload.Action, payload.PullRequest.HTMLURL)
		if payload.Action != "created" { // created is just a comment on the PR for some reason
			go PREvent(&payload)
		}
	case ghwebhooks.IssueCommentPayload:
		fmt.Println("received pull request comment, payload action & URL:", payload.Action, payload.Comment.HTMLURL)
		go PRCommentEvent(&payload)
	default:
		fmt.Println("missing handler for event", payload)
	}
	response.WriteHeader(http.StatusOK)
}

func GetV3Client() *v3.Client {
	return v3.NewClient(&http.Client{Transport: itr})
}

func PREvent(p *ghwebhooks.PullRequestPayload) {
	fmt.Println("New PR", p.PullRequest.HTMLURL)
	exists, err := doesRecordExist(p.Sender.Login)
	if err != nil {
		fmt.Println("error checking if record exists already:", err)
		return
	}
	if exists {
		return
	}
	msg := "Hello @" + p.Sender.Login + "!\n\n" +
		"Thanks for your pull request! Please read the Dogecoin CLA & COC here: https://example.com/TODO\n\n" +
		"Reply with 👍 to accept it."
	t := time.Now()
	P, _, err := GetV3Client().Issues.CreateComment(
		context.TODO(),
		p.Repository.Owner.Login,
		p.Repository.Name,
		int(p.Number),
		&v3.IssueComment{
			NodeID:    &p.Repository.NodeID,
			Body:      &msg,
			CreatedAt: &t,
			UpdatedAt: &t,
		},
	)
	if err != nil {
		fmt.Printf("making comment error: %v\n", err)
	}
	fmt.Println("Made comment: ", *P.HTMLURL)
}

func PRCommentEvent(p *ghwebhooks.IssueCommentPayload) {
	fmt.Println("Got PR comment event", p.Issue.HTMLURL)
	if p.Sender.Login == ghBotSlug+"[bot]" { // don't respond to self
		return
	}

	if p.Comment.Body == "👍" && p.Sender.Login == p.Issue.User.Login {
		exists, err := doesRecordExist(p.Sender.Login)
		if err != nil {
			fmt.Println("error checking if record exists already:", err)
			return
		}
		if exists {
			return
		}
		addApprovedGHUser(p.Sender.Login, p.Comment.HTMLURL)
		msg := "You have now accepted the CLA! @" + p.Sender.Login + " is now a shibe!"
		t := time.Now()
		reply, _, err := GetV3Client().Issues.CreateComment( // pr replies are sent through the issues api for some reason
			context.TODO(),
			p.Repository.Owner.Login,
			p.Repository.Name,
			int(p.Issue.Number),
			&v3.IssueComment{
				NodeID:    &p.Repository.NodeID,
				Body:      &msg,
				CreatedAt: &t,
				UpdatedAt: &t,
			},
		)
		if err != nil {
			fmt.Printf("making comment returned error: %v\n", err)
		}
		fmt.Println("Made reply: ", *reply.HTMLURL)
	}
}

// ToJSON is a convenience method for serializing to JSON.
func ToJSON(v interface{}) string {
	s, _ := json.MarshalIndent(v, "", "   ")
	return string(s)
}

func addApprovedGHUser(user string, messageLink string) {
	addToAirtable(user, "GitHub: Agreed to the CLA by messaging 👍 here: "+messageLink)
}

//for _, v := range records.Records {
//	var nameDup, notesDup interface{}
//	var ok bool
//	if nameDup, ok = v.Fields["Name"]; !ok {
//		continue
//	}
//	if notesDup, ok = v.Fields["Notes"]; ok {
//		continue
//	}
//	if nameDup == name && notesDup == notes { // already exists
//		return true, nil
//	}
//}
