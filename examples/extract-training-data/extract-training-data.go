package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/utils"
	"github.com/logrusorgru/aurora"
	zlog "github.com/rs/zerolog/log"
	"strings"
)

var pathChannel = make(chan []string, 10)
var dbHost = flag.String("db-host", "167.235.115.231", "database host")

func main() {
	termUi := false
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msgf("starting up...!")
	// it's a utility function, which does the following:
	// - it looks for all the messages with given string inside
	// - it collects all the inference steps which could lead to this messages
	db, err := storage.NewStorage(lg, *dbHost)
	if err != nil {
		lg.Fatal().Err(err).Msg("error initializing storage")
	}

	go func() {
		deDupe := make(map[string]struct{})
		chainsFound := 0
		for path := range pathChannel {
			pathJoined := strings.Join(path, ":")
			if _, exists := deDupe[pathJoined]; exists {
				continue
			}
			deDupe[pathJoined] = struct{}{}
			chainsFound++
			fmt.Printf("%d chains found, last chain is: %s\r",
				chainsFound, aurora.White(showLastN(pathJoined, 75)))
		}
	}()

	messages := make([]DbMessage, 0)
	err = db.Db.GetStructsSlice("get-messages-by-text", &messages,
		`%"final-report"%`)
	if err != nil {
		lg.Fatal().Err(err).Msg("error getting messages")
	}

	for _, msg := range messages {
		fmt.Printf("%s[%s] %s\n",
			aurora.BrightBlue(msg.ID),
			msg.Role,
			aurora.White(msg.Content))
		// now we need to find all paths which lead us here
		// we can get list of messages this message was a reply to
		getTheWayUp(db, msg.ID, []string{msg.ID})

		fmt.Printf("\n\n")
	}
}

func showLastN(joined string, n int) string {
	if len(joined) < n {
		return joined
	}

	// get last n runes of the string
	lastN := joined[len(joined)-n:]
	return "..." + lastN
}

func getTheWayUp(db *storage.Storage, id string, path []string) {
	if len(path) > 5 {
		pathChannel <- path
		return
	}

	shift := ""
	for i := 0; i < len(path); i++ {
		shift += " -- "
	}

	messageLinks := getListOfMessagesThisOneIsReplyTo(db, id)
	// ok, these are the messages we can serve as a reply to
	for _, prevMsg := range messageLinks {
		//fmt.Printf("%s%s\n", shift, aurora.BrightYellow(prevMsg))
		getTheWayUp(db, prevMsg, append(path, prevMsg))
	}

	if len(messageLinks) == 0 {
		//fmt.Printf("%s\n", aurora.BrightRed("terminal"))
		pathChannel <- path
	}
}

type DbMessage struct {
	ID      string `db:"id"`
	Name    string `db:"name"`
	Role    string `db:"role"`
	Content string `db:"content"`
}

type DbMessageLink struct {
	ID      string `db:"id"`
	ReplyTo string `db:"reply_to"`
}

func getListOfMessagesThisOneIsReplyTo(db *storage.Storage, id string) []string {
	messageLinks := make([]DbMessageLink, 0)
	err := db.Db.GetStructsSlice("get-message-links-by-id", &messageLinks, id)
	if err != nil {
		zlog.Fatal().Err(err).Msg("error getting message links")
	}

	result := make([]string, 0, len(messageLinks))
	for _, link := range messageLinks {
		result = append(result, link.ReplyTo)
	}

	return result
}
