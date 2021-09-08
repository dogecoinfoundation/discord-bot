package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

var (
	Token   string
	dg      *discordgo.Session

	shibe   *discordgo.Role

	msgIDToUser = make(map[string]*discordgo.User, 100)

	approvedUsers = make([]*discordgo.User, 0, 1000) //  TODO: replace with Airtable
	c = getCfg()
)

type config struct {
	Token string
	TargetRoleName string
	ServerID string
	ChannelID string
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

	dg, err = discordgo.New("Bot " + c.Token)
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}
	dg.AddHandler(memberAdd)
	dg.AddHandler(msgReact)
	dg.Identify.Intents = discordgo.IntentsAll

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
		return errors.New("no '"+c.TargetRoleName+"' role detected, you should add one")
	}
	fmt.Println("Found the role!")
	return nil
}

func memberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	fmt.Println(m.User.Username)
	msg, err := s.ChannelMessageSend(c.ChannelID, m.Member.Mention()+" has just joined my list of subjects! React with 👍 to accept the legal stuff, "+ m.Member.Mention()+". Much love, KS 3")
	if err != nil {
		fmt.Println("error:", err)
	}
	msgIDToUser[msg.ID] = m.User
}

func msgReact(s *discordgo.Session, m *discordgo.MessageReactionAdd) {
	if user, ok := msgIDToUser[m.MessageID]; ok {
		if m.UserID == user.ID && (m.Emoji.Name == "👍"){
			addApprovedUser(user)
			err := s.GuildMemberRoleAdd(c.ServerID, user.ID, shibe.ID)
			if err != nil {
				_, err = s.ChannelMessageSend(c.ChannelID, "OH NOOOOO i got an error: " + err.Error() + "\n Kindly slap Ishan")
				if err != nil {
					fmt.Println("Couldn't send a message. error: ", err)
				}
			}
		}
	}
}

func addApprovedUser(user *discordgo.User) {
	approvedUsers = append(approvedUsers, user)
	fmt.Println("added a new person. approved people are", approvedUsers)
}
