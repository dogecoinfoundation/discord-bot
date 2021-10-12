package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mehanizm/airtable"
)

var (
	dg    *discordgo.Session
	shibe *discordgo.Role
	table *airtable.Table

	c = getCfg()

	msgIDToUser = make(map[string]*discordgo.User, 100)
)

type config struct {
	Token          string
	TargetRoleName string
	ServerID       string
	ChannelID      string
	AirtableKey    string
	AirtableBaseID string
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

	fmt.Println("Bot is running! Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	dg.Close()
}

// finds the shibe role
func setup() error {
	roles, err := dg.GuildRoles(c.ServerID)
	if err != nil {
		return err
	}
	fmt.Println("Looking through server roles...")
	for i := range roles {
		if roles[i].Name == c.TargetRoleName {
			shibe = roles[i]
		}
	}
	if shibe == nil {
		return errors.New("no '" + c.TargetRoleName + "' role detected, you should add one")
	}
	fmt.Println("Found the role!")

	client := airtable.NewClient(c.AirtableKey)
	table = client.GetTable(c.AirtableBaseID, "Table 1")
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
	if user, ok := msgIDToUser[m.MessageID]; ok {
		if m.UserID == user.ID && (m.Emoji.Name == "👍") {
			addApprovedUser(user, messageToLink(m))
			_, err := s.ChannelMessageSend(c.ChannelID, "Wow, thanks for your acceptance, "+user.Mention()+". I'm adding the Shibe role yahoo!")
			if err != nil {
				fmt.Println("Couldn't send a message. error: ", err)
			}
			err = s.GuildMemberRoleAdd(c.ServerID, user.ID, shibe.ID)
			if err != nil {
				_, err = s.ChannelMessageSend(c.ChannelID, "OH NOOOOO I got an error while trying to make you a shibe: "+err.Error()+"\n Please send much stern messages to Ishan#9106")
				if err != nil {
					fmt.Println("Couldn't send a message. error: ", err)
				}
			}
		}
	}
}

func messageToLink(m *discordgo.MessageReactionAdd) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", m.GuildID, m.ChannelID, m.MessageID)
}

func addApprovedUser(user *discordgo.User, messageLink string) {
	addToAirtable(user.String(), "Discord: Agreed to the CLA by reacting with 👍 here: "+messageLink) // TODO: include some kind of message ID/link.
}

func addToAirtable(name string, notes string) {
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
